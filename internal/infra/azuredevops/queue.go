package azuredevops

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	AzdoFinalizerName     = "fr.simplified/azdo-finalizer"
	apiVersion            = "7.1"
	defaultRateLimit      = 5
	defaultMaxRetries     = 3
	defaultRetryBaseDelay = 1 * time.Second
	cacheExpiration       = 5 * time.Minute
)

type Client interface {
	GetQueueLength(ctx context.Context, poolName string) (int, error)
	// GetPoolIDByName retrieves the pool ID based on the provided pool name.
	GetPoolIDByName(ctx context.Context, poolName string) (int, error)
	// GetAgentsInPool fetches a list of agents associated with a specified pool ID.
	GetAgentsInPool(ctx context.Context, poolID int) ([]Agent, error)
	// DisableAgent disables an agent identified by the given pool ID and agent ID.
	DisableAgent(ctx context.Context, poolID int, agentID int) error
	// DeleteAgent deletes an agent specified by the pool ID and agent ID.
	DeleteAgent(ctx context.Context, poolID int, agentID int) error
}

type AzureDevOpsClient struct {
	PATToken    string
	OrgURL      string
	Project     string
	limiter     *rate.Limiter
	poolIDCache map[string]poolCacheEntry
	cacheMutex  sync.RWMutex
}

type poolCacheEntry struct {
	ID        int
	expiresAt time.Time
}

func NewAzureDevOpsClient(patToken, orgURL, project string) *AzureDevOpsClient {
	return &AzureDevOpsClient{
		PATToken:    strings.TrimSpace(patToken),
		OrgURL:      strings.TrimRight(orgURL, "/"),
		Project:     project,
		limiter:     rate.NewLimiter(rate.Limit(defaultRateLimit), 1),
		poolIDCache: make(map[string]poolCacheEntry),
	}
}

type QueueNameResponse struct {
	Count int         `json:"count"`
	Value []AgentPool `json:"value"`
}

func (a *AzureDevOpsClient) doRequestWithBackoff(ctx context.Context, req *http.Request) (*http.Response, error) {
	logger := log.FromContext(ctx)
	var resp *http.Response
	var err error

	// Wait for rate limiter
	if err := a.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter wait: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}

	for attempt := 0; attempt < defaultMaxRetries; attempt++ {
		resp, err = client.Do(req)

		if err != nil {
			if ctx.Err() == context.Canceled {
				return nil, fmt.Errorf("request canceled: %w", err)
			}

			// Calculate backoff with jitter
			backoff := defaultRetryBaseDelay * time.Duration(1<<attempt)
			jitter := time.Duration(rand.Int63n(int64(backoff / 2)))
			sleepTime := backoff + jitter

			select {
			case <-time.After(sleepTime):
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		// Check for rate limiting response
		if resp != nil && resp.StatusCode == http.StatusTooManyRequests {
			// Get retry-after if available
			retryAfter := resp.Header.Get("Retry-After")
			var sleepTime time.Duration

			if retryAfter != "" {
				retrySeconds, err := strconv.Atoi(retryAfter)
				if err == nil {
					sleepTime = time.Duration(retrySeconds) * time.Second
				}
			}

			if sleepTime == 0 {
				// Default backoff with jitter
				backoff := defaultRetryBaseDelay * time.Duration(1<<attempt)
				jitter := time.Duration(rand.Int63n(int64(backoff / 2)))
				sleepTime = backoff + jitter
			}

			// Close the response before waiting
			if resp.Body != nil {
				if err := resp.Body.Close(); err != nil {
					logger.Error(err, "Failed to close response body")
				}
			}

			select {
			case <-time.After(sleepTime):
				// Retry the request with a fresh client
				req = req.Clone(ctx)
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		// If we got here with a valid response, return it
		if resp != nil {
			return resp, nil
		}
	}

	return resp, err
}

type AgentPool struct {
	ID        int    `json:"id"`
	ProjectID string `json:"projectId"`
	Name      string `json:"name"`
	Pool      Pool   `json:"pool"`
}

type Pool struct {
	ID       int    `json:"id"`
	Scope    string `json:"scope"`
	Name     string `json:"name"`
	IsHosted bool   `json:"isHosted"`
	PoolType string `json:"poolType"`
	Size     int    `json:"size"`
	IsLegacy bool   `json:"isLegacy"`
	Options  string `json:"options"`
}

type QueueResponse struct {
	Count int              `json:"count"`
	Value []TaskAgentQueue `json:"value"`
}

type QueueItem struct {
	RequestID              int            `json:"requestId"`
	QueueTime              time.Time      `json:"queueTime"`
	AssignTime             *time.Time     `json:"assignTime"`
	ServiceOwner           string         `json:"serviceOwner"`
	HostID                 string         `json:"hostId"`
	ScopeID                string         `json:"scopeId"`
	PlanType               string         `json:"planType"`
	PlanID                 string         `json:"planId"`
	JobID                  string         `json:"jobId"`
	Demands                []string       `json:"demands"`
	MatchedAgents          []MatchedAgent `json:"matchedAgents"`
	Definition             Definition     `json:"definition"`
	Owner                  Owner          `json:"owner"`
	Data                   QueueItemData  `json:"data"`
	PoolID                 int            `json:"poolId"`
	OrchestrationID        string         `json:"orchestrationId"`
	MatchesAllAgentsInPool bool           `json:"matchesAllAgentsInPool"`
	Priority               int            `json:"priority"`
}

type MatchedAgent struct {
	Links             Links  `json:"_links"`
	ID                int    `json:"id"`
	Name              string `json:"name"`
	Version           string `json:"version"`
	Enabled           bool   `json:"enabled"`
	Status            string `json:"status"`
	ProvisioningState string `json:"provisioningState"`
}

type Definition struct {
	Links Links  `json:"_links"`
	ID    int    `json:"id"`
	Name  string `json:"name"`
}

type Owner struct {
	Links Links  `json:"_links"`
	ID    int    `json:"id"`
	Name  string `json:"name"`
}

type QueueItemData struct {
	ParallelismTag string `json:"ParallelismTag"`
	IsScheduledKey string `json:"IsScheduledKey"`
}

type Links struct {
	Self Link `json:"self"`
	Web  Link `json:"web"`
}

type Link struct {
	Href string `json:"href"`
}

type TaskAgentQueue struct {
	ID        int                    `json:"id"`
	Name      string                 `json:"name"`
	Pool      TaskAgentPoolReference `json:"pool"`
	ProjectID string                 `json:"projectId"`
}

type TaskAgentPoolReference struct {
	ID       int                  `json:"id"`
	IsHosted bool                 `json:"isHosted"`
	IsLegacy bool                 `json:"isLegacy"`
	Name     string               `json:"name"`
	Options  TaskAgentPoolOptions `json:"options"`
	PoolType string               `json:"poolType"`
	Scope    string               `json:"scope"`
	Size     int                  `json:"size"`
}

type TaskAgentPoolOptions string

func buildAuthHeader(patToken string) string {
	token := ":" + patToken
	encoded := base64.StdEncoding.EncodeToString([]byte(token))
	return "Basic " + encoded
}

func GetQueueIdFromName(ctx context.Context, orgURL, project, patToken, poolName string) (string, error) {
	logger := log.FromContext(ctx)

	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	url := fmt.Sprintf("%s/_apis/distributedtask/pools?poolName=%s&api-version=7.1",
		strings.TrimRight(orgURL, "/"), poolName)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", buildAuthHeader(patToken))

	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() == context.Canceled {
			return "", fmt.Errorf("request canceled while getting pool ID")
		}
		return "", fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			logger.Error(cerr, "Failed to close response body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("unexpected status code: %d - %s", resp.StatusCode, string(body))
	}

	var queueResp QueueNameResponse
	if err := json.NewDecoder(resp.Body).Decode(&queueResp); err != nil {
		return "", fmt.Errorf("failed to parse JSON response: %w", err)
	}

	if queueResp.Count == 0 || len(queueResp.Value) == 0 {
		return "", fmt.Errorf("no pool found with name: %s", poolName)
	}

	return fmt.Sprintf("%d", queueResp.Value[0].ID), nil
}

func (a *AzureDevOpsClient) GetQueueLength(ctx context.Context, poolName string) (int, error) {
	logger := log.FromContext(ctx)

	poolID, err := a.GetPoolIDByName(ctx, poolName)
	if err != nil {
		return 0, fmt.Errorf("failed to get pool ID: %w", err)
	}

	url := fmt.Sprintf("%s/_apis/distributedtask/pools/%d/jobrequests?api-version=%s",
		a.OrgURL, poolID, apiVersion)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", buildAuthHeader(a.PATToken))

	resp, err := a.doRequestWithBackoff(ctx, req)
	if err != nil {
		return 0, fmt.Errorf("failed to execute request: %w", err)
	}
	if errClose := resp.Body.Close(); errClose != nil {
		logger.Error(errClose, "Failed to close response body")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("unexpected status code: %d - %s", resp.StatusCode, string(body))
	}

	var queueResp struct {
		Count int         `json:"count"`
		Value []QueueItem `json:"value"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&queueResp); err != nil {
		return 0, fmt.Errorf("failed to parse response: %w", err)
	}

	// If there are no jobs at all, return 0
	if queueResp.Count == 0 || len(queueResp.Value) == 0 {
		logger.V(1).Info("No jobs in queue", "poolName", poolName)
		return 0, nil
	}

	// Count only unassigned jobs
	pendingJobs := 0
	for _, job := range queueResp.Value {
		if job.AssignTime == nil {
			pendingJobs++
		}
	}

	logger.Info("Queue status",
		"poolName", poolName,
		"totalJobs", queueResp.Count,
		"pendingJobs", pendingJobs)

	return pendingJobs, nil
}

func (a *AzureDevOpsClient) DeleteAgent(ctx context.Context, poolID int, agentID int) error {
	logger := log.FromContext(ctx)
	url := fmt.Sprintf("%s/_apis/distributedtask/pools/%d/agents/%d?api-version=%s",
		a.OrgURL, poolID, agentID, apiVersion)

	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", buildAuthHeader(a.PATToken))

	resp, err := a.doRequestWithBackoff(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}

	body, _ := io.ReadAll(resp.Body)
	if err := resp.Body.Close(); err != nil {
		logger.Error(err, "Failed to close response body")
	}

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent {
		return nil
	}

	if resp.StatusCode == http.StatusBadRequest && strings.Contains(string(body), "TaskAgentJobStillRunningException") {
		return fmt.Errorf("failed to delete agent as it is still running jobs")
	}

	return fmt.Errorf("unexpected status code: %d - %s", resp.StatusCode, string(body))
}

func (a *AzureDevOpsClient) DisableAgent(ctx context.Context, poolID int, agentID int) error {
	logger := log.FromContext(ctx)
	if agentID <= 0 {
		return fmt.Errorf("invalid agent ID: %d", agentID)
	}
	if poolID <= 0 {
		return fmt.Errorf("invalid pool ID: %d", poolID)
	}

	url := fmt.Sprintf("%s/_apis/distributedtask/pools/%d/agents/%d?api-version=%s",
		a.OrgURL, poolID, agentID, apiVersion)

	payload := struct {
		ID      int  `json:"id"`
		Enabled bool `json:"enabled"`
	}{
		ID:      agentID,
		Enabled: false,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PATCH", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", buildAuthHeader(a.PATToken))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	if errClose := resp.Body.Close(); errClose != nil {
		logger.Error(errClose, "Failed to close response body")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code: %d - %s", resp.StatusCode, string(body))
	}

	return nil
}

func (a *AzureDevOpsClient) GetAgentsInPool(ctx context.Context, poolID int) ([]Agent, error) {
	logger := log.FromContext(ctx)
	url := fmt.Sprintf("%s/_apis/distributedtask/pools/%d/agents?api-version=%s",
		a.OrgURL, poolID, apiVersion)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", buildAuthHeader(a.PATToken))

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	if errClose := resp.Body.Close(); errClose != nil {
		logger.Error(errClose, "Failed to close response body")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code: %d - %s", resp.StatusCode, string(body))
	}

	var agentResp struct {
		Count int     `json:"count"`
		Value []Agent `json:"value"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&agentResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return agentResp.Value, nil
}

func (a *AzureDevOpsClient) GetPoolIDByName(ctx context.Context, poolName string) (int, error) {
	logger := log.FromContext(ctx)

	// Check cache first
	a.cacheMutex.RLock()
	if entry, ok := a.poolIDCache[poolName]; ok && entry.expiresAt.After(time.Now()) {
		a.cacheMutex.RUnlock()
		return entry.ID, nil
	}
	a.cacheMutex.RUnlock()

	url := fmt.Sprintf("%s/_apis/distributedtask/pools?poolName=%s&api-version=%s",
		a.OrgURL, poolName, apiVersion)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", buildAuthHeader(a.PATToken))

	resp, err := a.doRequestWithBackoff(ctx, req)
	if err != nil {
		return 0, fmt.Errorf("failed to execute request: %w", err)
	}
	if errClose := resp.Body.Close(); errClose != nil {
		logger.Error(errClose, "Failed to close response body")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status code: %d - %s", resp.StatusCode, string(body))
	}

	var queueResp QueueNameResponse
	if err := json.Unmarshal(body, &queueResp); err != nil {
		return 0, fmt.Errorf("failed to parse response: %w", err)
	}

	if queueResp.Count == 0 || len(queueResp.Value) == 0 {
		return 0, fmt.Errorf("no pool found with name: %s", poolName)
	}

	poolID := queueResp.Value[0].ID

	// Cache the result
	a.cacheMutex.Lock()
	a.poolIDCache[poolName] = poolCacheEntry{
		ID:        poolID,
		expiresAt: time.Now().Add(cacheExpiration),
	}
	a.cacheMutex.Unlock()

	logger.V(1).Info("Retrieved pool ID", "poolName", poolName, "poolID", poolID)
	return poolID, nil
}
