package azuredevops

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/log"
)

type Client interface {
	GetQueueLength(ctx context.Context, poolName string) (int, error)
}

type AzureDevOpsClient struct {
	PATToken string
	OrgURL   string
	Project  string
}

func NewAzureDevOpsClient(patToken, orgURL, project string) *AzureDevOpsClient {
	return &AzureDevOpsClient{
		PATToken: strings.TrimSpace(patToken),
		OrgURL:   strings.TrimRight(orgURL, "/"),
		Project:  project,
	}
}

type QueueNameResponse struct {
	Count int         `json:"count"`
	Value []AgentPool `json:"value"`
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
	Count int         `json:"count"`
	Value []QueueItem `json:"value"`
}

type QueueItem struct {
	RequestID              int            `json:"requestId"`
	QueueTime              time.Time      `json:"queueTime"`
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

func buildAuthHeader(patToken string) string {
	token := ":" + patToken
	encoded := base64.StdEncoding.EncodeToString([]byte(token))
	return "Basic " + encoded
}

func GetQueueIdFromName(ctx context.Context, orgURL, project, patToken, poolName string) (string, error) {

	url := fmt.Sprintf("%s/_apis/distributedtask/pools?poolName=%s&api-version=7.2-preview.1",
		strings.TrimRight(orgURL, "/"), poolName)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("échec de la création de la requête pour récupérer l'ID de la queue: %w", err)
	}

	req.Header.Set("Authorization", buildAuthHeader(patToken))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("échec de l'exécution de la requête pour récupérer l'ID de la queue: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("statut inattendu %d et échec de la lecture du body: %w", resp.StatusCode, err)
		}
		return "", fmt.Errorf("statut inattendu lors de la récupération de la queue: %d - %s", resp.StatusCode, body)
	}

	var queueResp QueueNameResponse
	if err := json.NewDecoder(resp.Body).Decode(&queueResp); err != nil {
		return "", fmt.Errorf("échec de l'analyse de la réponse JSON: %w", err)
	}

	if queueResp.Count == 0 || len(queueResp.Value) == 0 {
		return "", fmt.Errorf("aucune queue trouvée dans la réponse")
	}

	return fmt.Sprintf("%d", queueResp.Value[0].ID), nil

}

func (a *AzureDevOpsClient) GetQueueLength(ctx context.Context, poolName string) (int, error) {
	logger := log.FromContext(ctx)

	poolID, err := GetQueueIdFromName(ctx, a.OrgURL, a.Project, a.PATToken, poolName)
	if err != nil {
		return 0, fmt.Errorf("échec de la récupération de l'ID du pool: %w", err)
	}
	logger.Info("Pool ID récupéré", "poolID", poolID)

	url := fmt.Sprintf("%s/_apis/distributedtask/pools/%s/jobrequests?api-version=6.0",
		a.OrgURL, poolID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return 0, fmt.Errorf("échec de la création de la requête: %w", err)
	}

	req.Header.Set("Authorization", buildAuthHeader(a.PATToken))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("échec de l'exécution de la requête: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Error(err, "failed to close response body: %v")
		}
	}()

	var queueResp QueueResponse
	if err := json.NewDecoder(resp.Body).Decode(&queueResp); err != nil {
		return 0, fmt.Errorf("échec de l'analyse de la réponse: %w", err)
	}

	logger.Info("Taille de la queue récupérée", "queueLength", queueResp.Count)
	return queueResp.Count, nil
}
