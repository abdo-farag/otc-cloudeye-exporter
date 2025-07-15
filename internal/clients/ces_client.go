package clients

import (
	"fmt"
	"github.com/abdo-farag/otc-cloudeye-exporter/internal/config"
	"github.com/abdo-farag/otc-cloudeye-exporter/internal/logs"  // Importing logs package
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	ces "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v1"
	cesv2 "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ces/v2"
)

// InitCESClient initializes CES v1 client with SafeBuild
func InitCESClient(cfg *config.Config, endpoint string, projectID string) (*ces.CesClient, error) {
	logs.Infof("Initializing CES v1 client for project: %s, endpoint: %s", projectID, endpoint)

	auth, _ := basic.NewCredentialsBuilder().
		WithAk(cfg.Auth.AccessKey).
		WithSk(cfg.Auth.SecretKey).
		WithProjectId(projectID).
		SafeBuild()

	hcClient, err := ces.CesClientBuilder().
		WithEndpoints([]string{endpoint}).
		WithCredential(auth).
		WithHttpConfig(config.GetHttpConfig().WithIgnoreSSLVerification(cfg.Global.IgnoreSSLVerify)).
		SafeBuild()
	if err != nil {
		logs.Errorf("Failed to build CES v1 client for project %s: %v", projectID, err)
		return nil, fmt.Errorf("failed to build CES v1 client: %w", err)
	}

	logs.Infof("CES v1 client successfully initialized for project: %s", projectID)

	return ces.NewCesClient(hcClient), nil
}

// InitCESv2Client initializes CES v2 client with SafeBuild
func InitCESv2Client(cfg *config.Config, endpoint string, projectID string) (*cesv2.CesClient, error) {
	logs.Infof("Initializing CES v2 client for project: %s, endpoint: %s", projectID, endpoint)

	auth, _ := basic.NewCredentialsBuilder().
		WithAk(cfg.Auth.AccessKey).
		WithSk(cfg.Auth.SecretKey).
		WithProjectId(projectID).
		SafeBuild()

	hcClient, err := cesv2.CesClientBuilder().
		WithEndpoints([]string{endpoint}).
		WithCredential(auth).
		WithHttpConfig(config.GetHttpConfig().WithIgnoreSSLVerification(cfg.Global.IgnoreSSLVerify)).
		SafeBuild()
	if err != nil {
		logs.Errorf("Failed to build CES v2 client for project %s: %v", projectID, err)
		return nil, fmt.Errorf("failed to build CES v2 client: %w", err)
	}

	logs.Infof("CES v2 client successfully initialized for project: %s", projectID)

	return cesv2.NewCesClient(hcClient), nil
}
