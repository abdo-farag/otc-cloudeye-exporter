package clients

import (
	"fmt"
	"github.com/abdo-farag/otc-cloudeye-exporter/internal/config"
	ces "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v1"
	cesv2 "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v2"
	evs "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/evs/v2"
	"github.com/abdo-farag/otc-cloudeye-exporter/internal/logs"
)

type Clients struct {
	CloudEyeV1 *ces.CesClient
	CloudEyeV2 *cesv2.CesClient
	RMS        *RmsClient
	EVS        *evs.EvsClient
	OBS        *ObsClient
}

// NewClientsWithEndpoints creates all service clients using static OTC endpoints
func NewClientsWithEndpoints(cfg *config.Config, epCfg *config.EndpointConfig) ([]*Clients, error) {
	var clientsList []*Clients
	region := epCfg.Region

	// Strictly require each endpoint to be in YAML!
	cesEndpoint, ok := epCfg.Services["SYS.CES"]
	if !ok {
		logs.Errorf("SYS.CES endpoint not defined in endpoints.yml!")
		return nil, fmt.Errorf("SYS.CES endpoint missing")
	}
	
	rmsEndpoint, ok := epCfg.Services["SYS.RMS"]
	if !ok {
		logs.Errorf("SYS.RMS endpoint not defined in endpoints.yml!")
		return nil, fmt.Errorf("SYS.RMS endpoint missing")
	}
	evsEndpoint, ok := epCfg.Services["SYS.EVS"]
	if !ok {
		logs.Errorf("SYS.EVS endpoint not defined in endpoints.yml!")
		return nil, fmt.Errorf("SYS.EVS endpoint missing")
	}
	obsEndpoint, ok := epCfg.Services["SYS.OBS"]
	if !ok {
		logs.Errorf("SYS.OBS endpoint not defined in endpoints.yml!")
		return nil, fmt.Errorf("SYS.OBS endpoint missing")
	}

	logs.Info("Initializing clients for region: ", region)

	for _, project := range cfg.Auth.Projects {
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

		evsClient, err := InitEVSClient(cfg, evsEndpoint, project.ID)
		if err != nil {
			logs.Errorf("❌ Failed init EVS client for project %s: %v", project.Name, err)
		}

		obsClient, err := NewObsClient(cfg, obsEndpoint)
		if err != nil {
			logs.Errorf("❌ Failed to init OBS client for project %s: %v", project.Name, err)
		}

		client := &Clients{
			CloudEyeV1: v1Client,
			CloudEyeV2: v2Client,
			RMS:        rmsClient,
			EVS:        evsClient,
			OBS:        obsClient,
		}
		clientsList = append(clientsList, client)
	}

	if len(clientsList) == 0 {
		logs.Errorf("No CES clients initialized successfully")
		logs.Flush()
		return nil, fmt.Errorf("no CES clients initialized successfully")
	}

	logs.Info("Successfully initialized clients")
	logs.Flush()
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

	if c.EVS != nil {
		logs.Info("Close EVS Client")
	}

	if c.OBS != nil {
		logs.Info("Close EVS Client")
	}
}
