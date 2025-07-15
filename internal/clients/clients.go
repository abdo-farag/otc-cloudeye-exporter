package clients

import (
	"fmt"
	"github.com/abdo-farag/otc-cloudeye-exporter/internal/config"
	ces "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v1"
	cesv2 "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v2"
	"github.com/abdo-farag/otc-cloudeye-exporter/internal/logs"
)

type Clients struct {
	CloudEyeV1 *ces.CesClient
	CloudEyeV2 *cesv2.CesClient
	RMS        *RmsClient
}

// NewClientsWithEndpoints creates all service clients using static OTC endpoints
func NewClientsWithEndpoints(cfg *config.Config, epCfg *config.EndpointConfig) ([]*Clients, error) {
	var clientsList []*Clients
	region := epCfg.Region
	cesEndpoint := fmt.Sprintf("https://ces.%s.otc.t-systems.com", region)
	rmsEndpoint := fmt.Sprintf("https://rms.%s.otc.t-systems.com", region)

	// Log the region and endpoints initialization
	logs.Info("Initializing clients for region: ", region)

	for _, project := range cfg.Auth.Projects {
		// Log project start
		logs.Info("Initializing clients for project: ", project.Name)

		v1Client, err := InitCESClient(cfg, cesEndpoint, project.ID)
		if err != nil {
			logs.Errorf("❌ Failed to init CES v1 for project %s: %v", project.Name, err)
			continue
		}

		v2Client, err := InitCESv2Client(cfg, cesEndpoint, project.ID)
		if err != nil {
			logs.Errorf("❌ Failed to init CES v2 for project %s: %v", project.Name, err)
			continue
		}

		rmsClient, err := InitRmsClient(cfg, rmsEndpoint, region)
		if err != nil {
			logs.Errorf("❌ Failed to init RMS client for project %s: %v", project.Name, err)
			continue
		}

		client := &Clients{
			CloudEyeV1: v1Client,
			CloudEyeV2: v2Client,
			RMS:        rmsClient,
		}

		clientsList = append(clientsList, client)
	}

	if len(clientsList) == 0 {
		logs.Errorf("No CES clients initialized successfully")
		logs.Flush()  // Ensure logs are flushed before exiting
		return nil, fmt.Errorf("no CES clients initialized successfully")
	}

	logs.Info("Successfully initialized clients")
	logs.Flush()  // Ensure logs are flushed after initialization
	return clientsList, nil
}

// Close releases resources associated with the Clients struct
func (c *Clients) Close() {
	// Close other clients similarly (RMS, CloudEye, etc.)
	if c.CloudEyeV1 != nil {
		// Add the CloudEye v1 cleanup logic here, if applicable
		logs.Info("Closed CloudEye V1 client")
	}

	if c.CloudEyeV2 != nil {
		// Add the CloudEye v2 cleanup logic here, if applicable
		logs.Info("Closed CloudEye V2 client")
	}

	if c.RMS != nil {
		// Add the RMS client cleanup logic here, if applicable
		logs.Info("Closed RMS client")
	}
}
