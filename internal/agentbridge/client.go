package agentbridge

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"forgegrid/internal/network"
)

type Client struct {
	BaseURL    string
	AgentName  string
	Token      string
	HTTPClient *http.Client
}

func NewClient(baseURL, agentName, token, fingerprint string, insecure bool) (*Client, error) {
	var tlsConfig *tls.Config
	if insecure {
		log.Println("WARNING: AgentBridge client is running in INSECURE mode. TLS verification is completely disabled!")
		tlsConfig = &tls.Config{InsecureSkipVerify: true}
	} else if fingerprint != "" {
		tlsConfig = network.PinTLSConfig(fingerprint)
	} else {
		return nil, fmt.Errorf("secure mode requires a TLS fingerprint")
	}

	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}
	return &Client{
		BaseURL:   baseURL,
		AgentName: agentName,
		Token:     token,
		HTTPClient: &http.Client{
			Transport: transport,
			Timeout:   10 * time.Second,
		},
	}, nil
}

func (c *Client) doReq(method, path string, body interface{}, out interface{}) error {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, c.BaseURL+path, bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.AgentName+":"+c.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b))
	}

	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (c *Client) SendMessage(recipient, taskID string, msgType MessageType, body string, ttl int, idempotencyKey string) (*AgentMessage, error) {
	req := map[string]interface{}{
		"recipient":       recipient,
		"task_id":         taskID,
		"type":            msgType,
		"body":            body,
		"ttl_seconds":     ttl,
		"idempotency_key": idempotencyKey,
	}
	var out AgentMessage
	if err := c.doReq(http.MethodPost, "/api/v1/agent-messages", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetInbox() ([]AgentMessage, error) {
	var out []AgentMessage
	if err := c.doReq(http.MethodGet, "/api/v1/agent-messages/inbox", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) Acknowledge(id string) (*AgentMessage, error) {
	var out AgentMessage
	if err := c.doReq(http.MethodPost, "/api/v1/agent-messages/"+id+"/acknowledge", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) Complete(id string, result json.RawMessage) (*AgentMessage, error) {
	req := map[string]interface{}{"result": result}
	var out AgentMessage
	if err := c.doReq(http.MethodPost, "/api/v1/agent-messages/"+id+"/complete", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) Fail(id string, result json.RawMessage) (*AgentMessage, error) {
	req := map[string]interface{}{"result": result}
	var out AgentMessage
	if err := c.doReq(http.MethodPost, "/api/v1/agent-messages/"+id+"/fail", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PostChannelMessage appends a message to a channel. Posting to a channel that
// does not exist yet creates it.
func (c *Client) PostChannelMessage(channel, body string) (*ChannelMessage, error) {
	path := "/api/v1/channels/" + url.PathEscape(channel) + "/messages"
	req := map[string]string{"body": body}
	var out ChannelMessage
	if err := c.doReq(http.MethodPost, path, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetChannelMessages returns channel messages after the given cursor, oldest
// first. A cursor of 0 returns from the start of the retained log.
func (c *Client) GetChannelMessages(channel string, since uint64, limit int) ([]ChannelMessage, error) {
	path := fmt.Sprintf("/api/v1/channels/%s/messages?since=%d&limit=%d",
		url.PathEscape(channel), since, limit)
	var out []ChannelMessage
	if err := c.doReq(http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// SetChannelPermissions sets a channel's access policy. Only the channel owner
// may call this; a channel with no owner is claimed by the caller.
func (c *Client) SetChannelPermissions(channel string, members []string, readOnly bool) (*ChannelMeta, error) {
	path := "/api/v1/channels/" + url.PathEscape(channel) + "/permissions"
	req := map[string]interface{}{"members": members, "read_only": readOnly}
	var out ChannelMeta
	if err := c.doReq(http.MethodPost, path, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListChannels returns every channel the relay knows about.
func (c *Client) ListChannels() ([]ChannelInfo, error) {
	var out []ChannelInfo
	if err := c.doReq(http.MethodGet, "/api/v1/channels", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}
