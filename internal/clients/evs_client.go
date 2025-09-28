package clients

import (
	"fmt"

	"github.com/abdo-farag/otc-cloudeye-exporter/internal/config"
	"github.com/abdo-farag/otc-cloudeye-exporter/internal/logs"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	evs "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/evs/v2"
	evsModel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/evs/v2/model"
)

// InitEVSClient initializes the EVS client for a specific project
func InitEVSClient(cfg *config.Config, endpoint string, projectID string) (*evs.EvsClient, error) {
	logs.Infof("Initializing EVS client for endpoint: %s, project: %s", endpoint, projectID)
	auth, _ := basic.NewCredentialsBuilder().
		WithAk(cfg.Auth.AccessKey).
		WithSk(cfg.Auth.SecretKey).
		WithProjectId(projectID).
		SafeBuild()
	hcClient, err := evs.EvsClientBuilder().
		WithEndpoints([]string{endpoint}).
		WithCredential(auth).
		WithHttpConfig(config.GetHttpConfig().WithIgnoreSSLVerification(cfg.Global.IgnoreSSLVerify)).
		SafeBuild()
	if err != nil {
		logs.Errorf("Failed to build EVS client: %v", err)
		return nil, fmt.Errorf("failed to build EVS client: %w", err)
	}
	logs.Infof("Successfully initialized EVS client for project: %s", projectID)
	return evs.NewEvsClient(hcClient), nil
}

// ListVolumes lists EVS volumes for the attached EVS client
func (c *Clients) ListVolumes() ([]evsModel.VolumeDetail, error) {
	logs.Debug("Listing EVS volumes...")
	limit := int32(1000)
	req := &evsModel.ListVolumesRequest{
		Limit: &limit,
	}
	resp, err := c.EVS.ListVolumes(req)
	if err != nil {
		logs.Errorf("Failed to list EVS volumes: %v", err)
		return nil, fmt.Errorf("failed to list EVS volumes: %w", err)
	}
	if resp.Volumes == nil {
		logs.Warn("ListVolumes response contains no volumes")
		return nil, nil
	}
	return *resp.Volumes, nil
}
