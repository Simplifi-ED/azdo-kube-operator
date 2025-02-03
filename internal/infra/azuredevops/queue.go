package azuredevops

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Client définit l'interface pour interagir avec Azure DevOps
type Client interface {
	GetQueueLength(ctx context.Context, poolName string) (int, error)
}

// AzureDevOpsClient implémente l'interface Client
type AzureDevOpsClient struct {
	PATToken string
	OrgURL   string
}

func NewAzureDevOpsClient(patToken, orgURL string) *AzureDevOpsClient {
	return &AzureDevOpsClient{
		PATToken: patToken,
		OrgURL:   orgURL,
	}
}

// QueueResponse structure pour parser la réponse de l'API Azure DevOps
type QueueResponse struct {
	Count int `json:"count"`
	// Ajoutez d'autres champs si nécessaire
}

func (a *AzureDevOpsClient) GetQueueLength(ctx context.Context, poolName string) (int, error) {
	logger := log.FromContext(ctx)

	// Construire l'URL de l'API Azure DevOps
	url := fmt.Sprintf("%s/_apis/distributedtask/pools/%s/jobrequests?api-version=6.0", a.OrgURL, poolName)

	// Créer la requête HTTP
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return 0, fmt.Errorf("échec de la création de la requête: %w", err)
	}

	// Ajouter l'authentification PAT
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", a.PATToken))

	// Exécuter la requête
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("échec de l'exécution de la requête: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("statut inattendu: %d", resp.StatusCode)
	}

	var queueResp QueueResponse
	if err := json.NewDecoder(resp.Body).Decode(&queueResp); err != nil {
		return 0, fmt.Errorf("échec de l'analyse de la réponse: %w", err)
	}

	logger.Info("Taille de la queue récupérée", "queueLength", queueResp.Count)
	return queueResp.Count, nil
}
