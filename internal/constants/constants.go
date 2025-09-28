package constants

import "time"

const (
	// CloudEye API defaults
	DefaultPeriod     = "1"
	DefaultLimit      = int32(1000)
	DefaultTimeWindow = time.Hour

	// OTC Namespaces - Compute
	NamespaceECS = "SYS.ECS"
	NamespaceAGT = "AGT.ECS"
	NamespaceBMS = "SERVICE.BMS"
	NamespaceAS  = "SYS.AS"

	// OTC Namespaces - Storage
	NamespaceEVS = "SYS.EVS"
	NamespaceOBS = "SYS.OBS"
	NamespaceSFS = "SYS.SFS"
	NamespaceEFS = "SYS.EFS"
	NamespaceCBR = "SYS.CBR"

	// OTC Namespaces - Network
	NamespaceVPC = "SYS.VPC"
	NamespaceELB = "SYS.ELB"
	NamespaceDC  = "SYS.DCAAS"
	NamespaceNAT = "SYS.NAT"
	NamespaceER  = "SYS.ER"
	NamespaceVPN = "SYS.VPN"

	// OTC Namespaces - Database
	NamespaceRDS       = "SYS.RDS"
	NamespaceDDS       = "SYS.DDS"
	NamespaceNoSQL     = "SYS.NoSQL"
	NamespaceGaussDB   = "SYS.GAUSSDB"
	NamespaceGaussDBV5 = "SYS.GAUSSDBV5"

	// OTC Namespaces - Security
	NamespaceWAF = "SYS.WAF"
	NamespaceCFW = "SYS.CFW"

	// OTC Namespaces - Application
	NamespaceDMS  = "SYS.DMS"
	NamespaceDCS  = "SYS.DCS"
	NamespaceAPIC = "SYS.APIC"

	// OTC Namespaces - Data Analysis
	NamespaceDWS  = "SYS.DWS"
	NamespaceES   = "SYS.ES"
	NamespaceDAYU = "SYS.DAYU"

	// Retry configuration
	MaxRetries        = 5
	InitialBackoff    = 5 * time.Second
	MaxBackoff        = 2 * time.Minute
	BackoffMultiplier = 2.0

	// Label keys
	LabelNamespace    = "namespace"
	LabelResourceID   = "resource_id"
	LabelResourceName = "resource_name"
	LabelProjectID    = "project_id"
	LabelProjectName  = "project_name"
	LabelUnit         = "unit"

	// Special resource IDs
	ResourceIDTotal   = "total"
	ResourceIDUnknown = "unknown"

	// Configuration defaults
	DefaultMetricPath = "/metrics"
	DefaultPort       = 9098
	DefaultHTTPSPort  = 9099

	// Default namespaces
	DefaultNamespaces = "SYS.ECS,SYS.EVS,SYS.RDS,SYS.ELB"

	ServiceTypeCompute      = "compute"
	ServiceTypeStorage      = "storage"
	ServiceTypeNetwork      = "network"
	ServiceTypeDatabase     = "database"
	ServiceTypeSecurity     = "security"
	ServiceTypeApplication  = "application"
	ServiceTypeDataAnalysis = "data_analysis"

	// HTTP/Proxy defaults
	DefaultProxySchema = "http"
	DefaultProxyPort   = 8080

	// Regions
	RegionEUDE = "eu-de"
	RegionEUNL = "eu-nl"
)

// OBS Operations
var OBSOperations = []string{
	"HEAD_OBJECT", "PUT_OBJECT", "DELETE_OBJECT", "GET_OBJECT",
	"PUT_PART", "POST_UPLOAD_INIT", "POST_UPLOAD_COMPLETE",
	"LIST_OBJECTS", "DELETE_OBJECTS", "COPY_OBJECT",
	"HEAD_BUCKET", "LIST_BUCKET_OBJECTS", "LIST_BUCKET_UPLOADS",
	"GET_BUCKET_LOCATION", "GET_BUCKET_POLICY", "PUT_BUCKET_POLICY",
	"DELETE_BUCKET_POLICY", "GET_BUCKET_ACL", "PUT_BUCKET_ACL",
}

// RetryableErrors contains error strings that should trigger retries
var RetryableErrors = []string{
	"408", "429", "500", "503", "timeout",
	"connection reset", "connection refused",
}

// AllNamespaces contains all supported OTC namespaces
var AllNamespaces = []string{
	// Compute
	NamespaceECS, NamespaceAGT, NamespaceBMS, NamespaceAS,

	// Storage
	NamespaceEVS, NamespaceOBS, NamespaceSFS, NamespaceEFS, NamespaceCBR,

	// Network
	NamespaceVPC, NamespaceELB, NamespaceDC, NamespaceNAT, NamespaceER, NamespaceVPN,

	// Database
	NamespaceRDS, NamespaceDDS, NamespaceNoSQL, NamespaceGaussDB, NamespaceGaussDBV5,

	// Security
	NamespaceWAF, NamespaceCFW,

	// Application
	NamespaceDMS, NamespaceDCS, NamespaceAPIC,

	// Data Analysis
	NamespaceDWS, NamespaceES, NamespaceDAYU,
}
