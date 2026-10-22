package txservice

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

// Roles a client can hold.
const (
	// RoleAgent may resolve names, prepare payments and read their state.
	// It may submit only payments that policy approved automatically. It is
	// the role an AI agent's runtime uses.
	RoleAgent = "agent"
	// RoleApprover may additionally approve and submit payments and revoke
	// delegations. It must be used only on an explicit user action.
	RoleApprover = "approver"
)

// Duration is a time.Duration written as a string such as "5m".
type Duration time.Duration

func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	parsed, err := time.ParseDuration(s)
	*d = Duration(parsed)
	return err
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

// Config configures the transaction service.
type Config struct {
	// NodeURI is the node the service talks to, e.g. http://127.0.0.1:9650.
	NodeURI string `json:"nodeURI"`
	// Listen is the address to serve on. It should be a loopback or private
	// address: the service is a back-end component.
	Listen string `json:"listen"`
	// DataDir holds the payment store and the audit log.
	DataDir     string             `json:"dataDir"`
	Clients     []ClientConfig     `json:"clients"`
	Delegations []DelegationConfig `json:"delegations"`

	// PreparedTTL is how long a prepared payment may wait for approval and
	// submission.
	PreparedTTL Duration `json:"preparedTTL"`
	// ExpiryMargin refuses recipients whose name expires within this
	// margin of wall-clock time.
	ExpiryMargin Duration `json:"expiryMargin"`
	// DecisionTimeout bounds how long submit waits for the network's
	// decision before returning a SUBMITTED payment.
	DecisionTimeout Duration `json:"decisionTimeout"`
	// ExplorerTxURL links receipts to a transaction record. "{txID}" is
	// replaced with the transaction ID.
	ExplorerTxURL string `json:"explorerTxURL"`

	RateLimits RateLimits `json:"rateLimits"`
}

// RateLimits apply per client.
type RateLimits struct {
	Read    RateLimit `json:"read"`
	Prepare RateLimit `json:"prepare"`
	Submit  RateLimit `json:"submit"`
}

// ClientConfig authorizes a client of the API.
type ClientConfig struct {
	Name string `json:"name"`
	// TokenSHA256 is the hex SHA-256 of the client's bearer token. The token
	// itself is never stored by the service.
	TokenSHA256 string   `json:"tokenSHA256"`
	Role        string   `json:"role"`
	Delegations []string `json:"delegations"`
}

// DelegationConfig is a user's delegation of a spending allowance to an
// agent.
type DelegationConfig struct {
	ID string `json:"id"`
	// Principal is the user's display name, normally their registered name.
	Principal string `json:"principal"`
	// PrincipalAddress is the user's V-chain address. The user's key is
	// never given to the service.
	PrincipalAddress string `json:"principalAddress"`
	// AgentName is the agent's display name, normally its registered name.
	AgentName string `json:"agentName"`
	// AgentKeyFile holds the agent's private key. It is created with mode
	// 0600 if missing.
	AgentKeyFile string `json:"agentKeyFile"`

	Assets []string `json:"assets"`
	// Limits are decimal amounts of the asset.
	PerPaymentMax  string `json:"perPaymentMax"`
	DailyMax       string `json:"dailyMax"`
	AutoApproveMax string `json:"autoApproveMax"`
	// NamedRecipientsOnly refuses payments to raw addresses.
	NamedRecipientsOnly bool       `json:"namedRecipientsOnly"`
	NotAfter            *time.Time `json:"notAfter,omitempty"`
}

var errConfig = errors.New("invalid configuration")

// LoadConfig reads a JSON configuration file and applies defaults.
func LoadConfig(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	config := &Config{}
	if err := json.Unmarshal(b, config); err != nil {
		return nil, fmt.Errorf("%w: %s", errConfig, err)
	}
	config.applyDefaults()
	return config, config.validate()
}

func (c *Config) applyDefaults() {
	if c.PreparedTTL == 0 {
		c.PreparedTTL = Duration(5 * time.Minute)
	}
	if c.ExpiryMargin == 0 {
		c.ExpiryMargin = Duration(10 * time.Minute)
	}
	if c.DecisionTimeout == 0 {
		c.DecisionTimeout = Duration(15 * time.Second)
	}
	if c.RateLimits.Read.PerMinute == 0 {
		c.RateLimits.Read = RateLimit{PerMinute: 120, Burst: 30}
	}
	if c.RateLimits.Prepare.PerMinute == 0 {
		c.RateLimits.Prepare = RateLimit{PerMinute: 10, Burst: 5}
	}
	if c.RateLimits.Submit.PerMinute == 0 {
		c.RateLimits.Submit = RateLimit{PerMinute: 5, Burst: 3}
	}
}

func (c *Config) validate() error {
	if c.NodeURI == "" || c.DataDir == "" {
		return fmt.Errorf("%w: nodeURI and dataDir are required", errConfig)
	}
	delegations := map[string]bool{}
	for _, d := range c.Delegations {
		if d.ID == "" || delegations[d.ID] {
			return fmt.Errorf("%w: delegation IDs must be unique and non-empty", errConfig)
		}
		delegations[d.ID] = true
		if d.AgentKeyFile == "" || d.PrincipalAddress == "" {
			return fmt.Errorf("%w: delegation %q needs agentKeyFile and principalAddress", errConfig, d.ID)
		}
	}
	for _, client := range c.Clients {
		if client.Role != RoleAgent && client.Role != RoleApprover {
			return fmt.Errorf("%w: client %q has unknown role %q", errConfig, client.Name, client.Role)
		}
		if len(client.TokenSHA256) != 2*sha256.Size {
			return fmt.Errorf("%w: client %q needs a hex SHA-256 token hash", errConfig, client.Name)
		}
		for _, id := range client.Delegations {
			if !delegations[id] {
				return fmt.Errorf("%w: client %q refers to unknown delegation %q", errConfig, client.Name, id)
			}
		}
	}
	return nil
}

// HashToken returns the value of ClientConfig.TokenSHA256 for [token].
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// authenticate returns the client holding [token].
func (c *Config) authenticate(token string) (*ClientConfig, bool) {
	if token == "" {
		return nil, false
	}
	hash := HashToken(token)
	var found *ClientConfig
	for i := range c.Clients {
		// Compare against every client in constant time.
		if subtle.ConstantTimeCompare([]byte(hash), []byte(c.Clients[i].TokenSHA256)) == 1 {
			found = &c.Clients[i]
		}
	}
	return found, found != nil
}

func (c *ClientConfig) mayUse(delegation string) bool {
	for _, id := range c.Delegations {
		if id == delegation {
			return true
		}
	}
	return false
}
