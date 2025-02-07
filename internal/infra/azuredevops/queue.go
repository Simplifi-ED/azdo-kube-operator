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

// Client définit l'interface pour interagir avec Azure DevOps.
type Client interface {
	GetQueueLength(ctx context.Context, poolName string) (int, error)
}

// AzureDevOpsClient implémente l'interface Client.
type AzureDevOpsClient struct {
	PATToken string
	OrgURL   string
	Project  string
}

// NewAzureDevOpsClient crée et retourne un nouvel AzureDevOpsClient.
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

// QueueResponse represents the top-level response.
type QueueResponse struct {
	Count int         `json:"count"`
	Value []QueueItem `json:"value"`
}

// QueueItem represents each item in the queue.
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

// MatchedAgent represents an agent matched to the queue item.
type MatchedAgent struct {
	Links             Links  `json:"_links"`
	ID                int    `json:"id"`
	Name              string `json:"name"`
	Version           string `json:"version"`
	Enabled           bool   `json:"enabled"`
	Status            string `json:"status"`
	ProvisioningState string `json:"provisioningState"`
}

// Definition represents the build or job definition.
type Definition struct {
	Links Links  `json:"_links"`
	ID    int    `json:"id"`
	Name  string `json:"name"`
}

// Owner represents the owner information.
type Owner struct {
	Links Links  `json:"_links"`
	ID    int    `json:"id"`
	Name  string `json:"name"`
}

// QueueItemData represents additional data for the queue item.
type QueueItemData struct {
	ParallelismTag string `json:"ParallelismTag"`
	IsScheduledKey string `json:"IsScheduledKey"`
}

// Links wraps the self and web links.
type Links struct {
	Self Link `json:"self"`
	Web  Link `json:"web"`
}

// Link represents a hyperlink with an href.
type Link struct {
	Href string `json:"href"`
}

// buildAuthHeader génère l'en-tête d'authentification Basic pour Azure DevOps.
func buildAuthHeader(patToken string) string {
	token := ":" + patToken
	encoded := base64.StdEncoding.EncodeToString([]byte(token))
	return "Basic " + encoded
}

// GetQueueIdFromName récupère l'identifiant de la file (ou pool) à partir de son nom.
func GetQueueIdFromName(ctx context.Context, orgURL, project, patToken, poolName string) (string, error) {
	// logger := log.FromContext(ctx)
	// Note : Vérifiez que l'URL correspond à celle documentée par Azure DevOps.
	// url := fmt.Sprintf("%s/_apis/distributedtask/queues?queueName=%s&api-version=6.0-preview.1",
	// strings.TrimRight(orgURL, "/"), poolName)
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

	// Désérialisation dans la structure QueueNameResponse.
	var queueResp QueueNameResponse
	if err := json.NewDecoder(resp.Body).Decode(&queueResp); err != nil {
		return "", fmt.Errorf("échec de l'analyse de la réponse JSON: %w", err)
	}

	// Vérification que des données ont bien été retournées.
	if queueResp.Count == 0 || len(queueResp.Value) == 0 {
		return "", fmt.Errorf("aucune queue trouvée dans la réponse")
	}

	// Par exemple, on retourne l'ID du premier AgentPool trouvé.
	// logger.Info("L'id a été trouvé ! %d", queueResp.Value[0].ID)
	return fmt.Sprintf("%d", queueResp.Value[0].ID), nil

}

// GetQueueLength retourne la longueur de la file d'attente pour le pool spécifié.
func (a *AzureDevOpsClient) GetQueueLength(ctx context.Context, poolName string) (int, error) {
	logger := log.FromContext(ctx)

	// Optionnel : récupération de l'ID de la queue si nécessaire.
	// Ici, on suppose que le nom du pool suffit et on se base sur ce nom dans l'URL.
	// Si votre API nécessite un poolId récupéré via GetQueueIdFromName, décommentez ce bloc.

	poolID, err := GetQueueIdFromName(ctx, a.OrgURL, a.Project, a.PATToken, poolName)
	if err != nil {
		return 0, fmt.Errorf("échec de la récupération de l'ID du pool: %w", err)
	}
	logger.Info("Pool ID récupéré", "poolID", poolID)

	// Construire l'URL pour récupérer les informations du pool.
	// Note : Vérifiez l'endpoint correct dans la documentation Azure DevOps.
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
	defer resp.Body.Close()

	// body, _ := io.ReadAll(resp.Body)
	// if resp.StatusCode != http.StatusOK {
	// 	return 0, fmt.Errorf("statut inattendu: %d - %s", resp.StatusCode, body)
	// }
	// logger.Info("Raw response", "body", string(body))

	var queueResp QueueResponse
	if err := json.NewDecoder(resp.Body).Decode(&queueResp); err != nil {
		return 0, fmt.Errorf("échec de l'analyse de la réponse: %w", err)
	}

	logger.Info("Taille de la queue récupérée", "queueLength", queueResp.Count)
	return queueResp.Count, nil
}
