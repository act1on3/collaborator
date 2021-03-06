package collaborator

import (
	"encoding/json"
	"io/ioutil"
	"math/rand"
	"sync"
	"time"

	"github.com/miekg/dns"
	"github.com/projectdiscovery/retryablehttp-go"
)

const BURP_URL = "https://collaborator4polling.act1on3.site/burpresults?biid="

type BurpCollaborator struct {
	sync.RWMutex
	BurpURl        string
	MaxBufferLimit int
	RespBuffer     []BurpHTTPResponse
	BIIDs          map[string]struct{}
	Subdomains     []string
	client         *retryablehttp.Client
}

func NewBurpCollaborator() *BurpCollaborator {
	retryablehttp := retryablehttp.NewClient(retryablehttp.DefaultOptionsSingle)

	return &BurpCollaborator{
		BIIDs:  make(map[string]struct{}),
		client: retryablehttp,
	}
}

func (b *BurpCollaborator) AddSubdomain(subdomain string) {
	b.Lock()
	defer b.Unlock()

	b.Subdomains = append(b.Subdomains, subdomain)
}

func (b *BurpCollaborator) AddSubdomains(subdomains []string) {
	b.Lock()
	defer b.Unlock()

	b.Subdomains = append(b.Subdomains, subdomains...)
}

func (b *BurpCollaborator) AddBIID(biid string) {
	b.Lock()
	defer b.Unlock()

	b.BIIDs[biid] = struct{}{}
}

func (b *BurpCollaborator) AddBIIDs(biids []string) {
	for _, biid := range biids {
		b.AddBIID(biid)
	}
}

func (b *BurpCollaborator) One() string {
	b.RLock()
	defer b.RUnlock()

	return b.Subdomains[rand.Intn(len(b.Subdomains))]
}

func (b *BurpCollaborator) Poll() error {
	if b.BurpURl == "" {
		b.BurpURl = BURP_URL
	}
	for biid := range b.BIIDs {
		b.poll(biid)
	}

	return nil
}

func (b *BurpCollaborator) PollEach(t time.Duration) {
	for range time.Tick(t) {
		if len(b.BIIDs) == 0 {
			return
		}

		b.Poll()
	}
}

func (b *BurpCollaborator) PollById(id string) error {
	_, err := b.poll(id)
	if err != nil {
		return err
	}

	return nil
}

func (b *BurpCollaborator) poll(id string) (*BurpHTTPResponse, error) {
	resp, err := b.client.Get(b.BurpURl + id)
	if err != nil {
		return nil, err
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var burpHttpResp BurpHTTPResponse
	err = json.Unmarshal(data, &burpHttpResp)
	if err != nil {
		return nil, err
	}

	var r *BurpResponse
	for i := range burpHttpResp.Responses {
		r = &burpHttpResp.Responses[i]
		r.Data.RawRequestDecoded, _ = Base64Decode(r.Data.RawRequest)
		r.Data.RequestDecoded, _ = Base64Decode(r.Data.Request)
		r.Data.ResponseDecoded, _ = Base64Decode(r.Data.Response)
		if r.Data.Type > 0 {
			r.Data.RequestType = dns.TypeToString[uint16(r.Data.Type)]
		}
		r.Data.SenderDecoded, _ = Base64Decode(r.Data.Sender)
		for _, recipient := range r.Data.Recipients {
			recipientbase64Decoded, _ := Base64Decode(recipient)
			r.Data.RecipientsDecoded = append(r.Data.RecipientsDecoded, recipientbase64Decoded)
		}
		r.Data.MessageDecoded, _ = Base64Decode(r.Data.Message)
		r.Data.ConversationDecoded, _ = Base64Decode(r.Data.Conversation)
	}

	b.Lock()
	if b.MaxBufferLimit > 0 && len(b.RespBuffer) >= b.MaxBufferLimit {
		// evict oldest response
		b.RespBuffer = b.RespBuffer[len(b.RespBuffer)-b.MaxBufferLimit:]
	}
	b.RespBuffer = append(b.RespBuffer, burpHttpResp)
	defer b.Unlock()

	return &burpHttpResp, nil
}

func (b *BurpCollaborator) Empty() {
	b.Lock()
	defer b.Unlock()

	b.RespBuffer = make([]BurpHTTPResponse, 0)
}

type BurpHTTPResponse struct {
	Responses []BurpResponse `json:"responses,omitempty"`
}

type BurpResponse struct {
	Protocol          string           `json:"protocol,omitempty"`
	OpCode            string           `json:"opCode,omitempty"`
	InteractionString string           `json:"interactionString,omitempty"`
	ClientPart        string           `json:"clientPart,omitempty"`
	Data              BurpResponseData `json:"data,omitempty"`
	Time              string           `json:"time,omitempty"`
	Client            string           `json:"client,omitempty"`
}

type BurpResponseData struct {
	SubDomain           string `json:"subDomain,omitempty"`
	Type                int    `json:"type,omitempty"`
	RequestType         string
	RawRequest          string `json:"rawRequest,omitempty"`
	RawRequestDecoded   string
	Request             string `json:"request,omitempty"`
	RequestDecoded      string
	Response            string `json:"response,omitempty"`
	ResponseDecoded     string
	Sender              string `json:"sender,omitempty"`
	SenderDecoded       string
	Recipients          []string `json:"recipients,omitempty"`
	RecipientsDecoded   []string
	Message             string `json:"message,omitempty"`
	MessageDecoded      string
	Conversation        string `json:"conversation,omitempty"`
	ConversationDecoded string
}
