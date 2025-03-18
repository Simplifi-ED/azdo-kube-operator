package azuredevops

const (
	AzdoFinalizerName = "fr.simplified/azdo-finalizer"
	ApiVersion        = "7.1"
)

type Agent struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Enabled bool   `json:"enabled"`
	PoolID  int    `json:"poolId"`
}
