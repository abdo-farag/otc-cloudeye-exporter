# otc-cloudeye-exporter

A robust Prometheus exporter for Open Telekom Cloud (OTC) Cloud Eye metrics with enterprise-grade features including multi-project support, dynamic namespace selection, and comprehensive logging.

## 🌟 Key Features

- **Multi-Project Architecture**: Simultaneously collect metrics from multiple OTC projects with automatic project validation
- **Dynamic Namespace Selection**: Configure namespaces statically or override them dynamically via HTTP query parameters
- **Enterprise Logging**: File-based logging with rotation, compression, and configurable levels
- **Flexible Deployment**: Support for both HTTP and HTTPS with customizable ports and TLS configuration
- **Service Endpoint Mapping**: Configurable API endpoints for different OTC services and regions
- **Error Resilience**: Graceful handling of project validation failures and missing endpoints
- **Resource Management**: Automatic cleanup of clients and connections on shutdown

## 📋 Prerequisites

- Go 1.19+ (for building from source)
- Valid OTC credentials (Access Key & Secret Key)
- Network access to OTC Cloud Eye APIs

## 🚀 Quick Start

### Installation

```bash
git clone https://github.com/abdo-farag/otc-cloudeye-exporter.git
cd otc-cloudeye-exporter
go build -o otc-cloudeye-exporter
```

### Basic Configuration

Create the required configuration files in your working directory:

#### 1. `clouds.yml` - Main Configuration

```yaml
auth:
  access_key: "your_otc_access_key"
  secret_key: "your_otc_secret_key" 
  region: "eu-de"
  # If no project set it will scrap all project for the selected namespace/s
  projects:
    - name: "eu-de_prod"
    - name: "eu_de_staging"
    - name: "eu_de_dev"
    - name: "eu_de"

global:
  metric_path: "/metrics"
  port: 9098
  https_port: 9099
  enable_https: false
  tls_cert: "/path/to/cert.pem"
  tls_key: "/path/to/key.pem"
  namespaces: "SYS.ECS,SYS.EVS,SYS.RDS,SYS.ELB"
  ignore_ssl_verify: true
```

#### 2. `endpoints.yml` - Service Endpoints

```yaml
region: "eu-de"
services:
  SYS.ECS: "https://ecs.{eu-de}.otc.t-systems.com"
  SYS.EVS: "https://evs.{eu-de}.otc.t-systems.com" 
  SYS.RDS: "https://rds.{eu-de}.otc.t-systems.com"
  SYS.ELB: "https://elb.{eu-de}.otc.t-systems.com"
  SYS.VPC: "https://vpc.{eu-de}.otc.t-systems.com"
  SYS.OBS: "https://obs.{eu-de}.otc.t-systems.com"
```

#### 3. `logs.yml` - Logging Configuration

```yaml
logging:
  - type: FILE
    level: info
    file:
      enabled: true
      filename: "otc-exporter.log"
      encoder: json
      max_size: 10485760    # 10MB
      max_backups: 5
      max_age: 30           # days
      compress: true
  - type: CONSOLE
    level: info
    console:
      enabled: true
      encoder: console
```

### Modify the env file with your tenant values
```bash
cat > .env << 'EOF'
OS_DOMAIN_ID=your_domain_id_here
OS_DOMAIN_NAME=your_domain_name_here
OS_ACCESS_KEY=your_access_key_here
OS_SECRET_KEY=your_secret_key_here
EOF

source .env
```

### Running the Exporter

```
./otc-cloudeye-exporter -config ./clouds.yml
```

## 📊 Usage

### Accessing Metrics

- **Default namespaces**: `http://localhost:9098/metrics`
- **Custom namespaces**: `http://localhost:9098/metrics?ns=SYS.ECS,SYS.RDS`
- **HTTPS (if enabled)**: `https://localhost:9099/metrics`

### Supported OTC Namespaces

The exporter can collect metrics from **any OTC Cloud Eye namespace**. Configure the namespaces you need in your `clouds.yml` or specify them dynamically via query parameters.

**Common OTC Namespaces and Services Interconnected with Cloud Eye**:

| Category        | Service                                             | Namespace        | Reference |
|-----------------|-----------------------------------------------------|------------------|-----------|
| **Compute**     | Elastic Cloud Server                                | `SYS.ECS`        | [ECS Metrics](https://docs.otc.t-systems.com/cloud-eye/api-ref/apis_for_querying_metrics/querying_metrics.html#querying-metrics) |
|                 | Elastic Cloud Server Agent based Metrics            | `AGT.ECS`        | [ECS Metrics](https://docs.otc.t-systems.com/cloud-eye/api-ref/apis_for_querying_metrics/querying_metrics.html#querying-metrics) |
|                 | Bare Metal Server                                   | `SERVICE.BMS`    | [BMS Metrics](https://docs.otc.t-systems.com/cloud-eye/api-ref/apis_for_querying_metrics/querying_metrics.html#querying-metrics) |
|                 | Auto Scaling                                        | `SYS.AS`         | [AS Metrics](https://docs.otc.t-systems.com/cloud-eye/api-ref/apis_for_querying_metrics/querying_metrics.html#querying-metrics)  |
| **Storage**     | Elastic Volume Service (attached to an ECS or BMS)  | `SYS.EVS`        | [EVS Metrics](https://docs.otc.t-systems.com/cloud-eye/api-ref/apis_for_querying_metrics/querying_metrics.html#querying-metrics) |
|                 | Object Storage Service                              | `SYS.OBS`        | [OBS Metrics](https://docs.otc.t-systems.com/cloud-eye/api-ref/apis_for_querying_metrics/querying_metrics.html#querying-metrics) |
|                 | Scalable File Service                               | `SYS.SFS`        | [SFS Metrics](https://docs.otc.t-systems.com/cloud-eye/api-ref/apis_for_querying_metrics/querying_metrics.html#querying-metrics) |
|                 | SFS Turbo                                           | `SYS.EFS`        | [SFS Turbo Metrics](https://docs.otc.t-systems.com/cloud-eye/api-ref/apis_for_querying_metrics/querying_metrics.html#querying-metrics) |
|                 | Cloud Backup and Recovery                           | `SYS.CBR`        | [CBR Metrics](https://docs.otc.t-systems.com/cloud-eye/api-ref/apis_for_querying_metrics/querying_metrics.html#querying-metrics) |
| **Network**     | Elastic IP and bandwidth                            | `SYS.VPC`        | [VPC Metrics](https://docs.otc.t-systems.com/cloud-eye/api-ref/apis_for_querying_metrics/querying_metrics.html#querying-metrics) |
|                 | Elastic Load Balance                                | `SYS.ELB`        | [ELB Metrics](https://docs.otc.t-systems.com/cloud-eye/api-ref/apis_for_querying_metrics/querying_metrics.html#querying-metrics) |
|                 | Direct Connect                                      | `SYS.DCAAS`      | [Direct Connect Metrics](https://docs.otc.t-systems.com/cloud-eye/api-ref/apis_for_querying_metrics/querying_metrics.html#querying-metrics) |
|                 | NAT Gateway                                         | `SYS.NAT`        | [NAT Gateway Metrics](https://docs.otc.t-systems.com/cloud-eye/api-ref/apis_for_querying_metrics/querying_metrics.html#querying-metrics) |
|                 | Enterprise Router                                   | `SYS.ER`         | [Enterprise Router Metrics](https://docs.otc.t-systems.com/enterprise-router/umn/monitoring_and_auditing/cloud_eye_monitoring/supported_metrics) |
|                 | Virtual Private Network                             | `SYS.VPN`        | [VPN Metrics](https://docs.otc.t-systems.com/cloud-eye/api-ref/apis_for_querying_metrics/querying_metrics.html#querying-metrics) |
| **Security**    | Web Application Firewall                            | `SYS.WAF`        | [WAF Metrics](https://docs.otc.t-systems.com/cloud-eye/api-ref/apis_for_querying_metrics/querying_metrics.html#querying-metrics) |
|                 | Cloud Firewall                                      | `SYS.CFW`        | [CFW Metrics](https://docs.otc.t-systems.com/cloud-firewall/umn/using_cloud_eye_to_monitor_cfw/cfw_monitored_metrics) |
| **Application** | Distributed Message Service                         | `SYS.DMS`        | [DMS Metrics](https://docs.otc.t-systems.com/cloud-eye/api-ref/apis_for_querying_metrics/querying_metrics.html#querying-metrics) |
|                 | Distributed Cache Service                           | `SYS.DCS`        | [DCS Metrics](https://docs.otc.t-systems.com/cloud-eye/api-ref/apis_for_querying_metrics/querying_metrics.html#querying-metrics) |
|                 | API Gateway                                         | `SYS.APIC`       | [API Gateway Metrics](https://docs.otc.t-systems.com/cloud-eye/api-ref/apis_for_querying_metrics/querying_metrics.html#querying-metrics) |
| **Database**    | Relational Database Service                         | `SYS.RDS`        | [RDS Metrics](https://docs.otc.t-systems.com/cloud-eye/api-ref/apis_for_querying_metrics/querying_metrics.html#querying-metrics) |
|                 | Document Database Service                           | `SYS.DDS`        | [DDS Metrics](https://docs.otc.t-systems.com/cloud-eye/api-ref/apis_for_querying_metrics/querying_metrics.html#querying-metrics) |
|                 | GeminiDB                                            | `SYS.NoSQL`      | [GeminiDB Metrics](https://docs.otc.t-systems.com/cloud-eye/api-ref/apis_for_querying_metrics/querying_metrics.html#querying-metrics) |
|                 | GaussDB(for MySQL)                                  | `SYS.GAUSSDB`    | [GaussDB MySQL Metrics](https://docs.otc.t-systems.com/cloud-eye/api-ref/apis_for_querying_metrics/querying_metrics.html#querying-metrics) |
|                 | GaussDB(for openGauss)                              | `SYS.GAUSSDBV5`  | [GaussDB openGauss Metrics](https://docs.otc.t-systems.com/cloud-eye/api-ref/apis_for_querying_metrics/querying_metrics.html#querying-metrics) |
| **Data Analysis** | Data Warehouse Service                            | `SYS.DWS`        | [DWS Metrics](https://docs.otc.t-systems.com/cloud-eye/api-ref/apis_for_querying_metrics/querying_metrics.html#querying-metrics) |
|                 | Cloud Search Service                                | `SYS.ES`         | [Elasticsearch Metrics](https://docs.otc.t-systems.com/cloud-eye/api-ref/apis_for_querying_metrics/querying_metrics.html#querying-metrics) |
|                 | Data Ingestion Service (DIS)                        | `SYS.DAYU`       | [DIS Metrics](https://docs.otc.t-systems.com/cloud-eye/api-ref/apis_for_querying_metrics/querying_metrics.html#querying-metrics) |

**Note**: This list is not exhaustive. The exporter supports any namespace available in OTC Cloud Eye. Simply add the desired namespaces to your configuration or endpoint mappings.

## 🔧 Advanced Configuration

### HTTPS Configuration

```yaml
global:
  enable_https: true
  https_port: 9099
  tls_cert: "/etc/ssl/certs/exporter.crt"
  tls_key: "/etc/ssl/private/exporter.key"
```

### Multi-Region Setup

For multiple regions, run separate exporter instances with different configurations:

```yaml
# eu-de-config.yml
auth:
  region: "eu-de"
  # ... other config

# eu-nl-config.yml  
auth:
  region: "eu-nl"
  # ... other config
```

### Project Validation

The exporter automatically validates that configured projects exist in the specified region before initializing clients. Invalid projects are logged and skipped, allowing the exporter to continue with valid projects.

## 🔍 Monitoring & Observability

### Logs

The exporter provides detailed logging including:
- Project validation results
- Client initialization status
- Metric collection activities
- Error conditions and warnings

### Health Check

The exporter serves on the configured port and will respond to health checks on the metrics endpoint.

## 🐳 Docker Deployment

```bash
docker run --rm --name cloudeye-exporter -p 9098:9098 -p 9099:9099 \
--env-file .env \
ghcr.io/abdo-farag/otc-cloudeye-exporter:latest
```

### Or
```bash
source .env

docker run --rm --name cloudeye-exporter -p 9098:9098 -p 9099:9099 \
  -e OS_DOMAIN_ID=${OS_DOMAIN_ID} \
  -e OS_DOMAIN_NAME=${OS_DOMAIN_NAME} \
  -e OS_ACCESS_KEY=${OS_ACCESS_KEY} \
  -e OS_SECRET_KEY=${OS_SECRET_KEY} \
  ghcr.io/abdo-farag/otc-cloudeye-exporter:latest
```

### using env file


## 📈 Prometheus Integration

### Prometheus Configuration

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'otc-cloudeye'
    static_configs:
      - targets: ['localhost:9098']
    scrape_interval: 60s
    metrics_path: '/metrics'
    params:
      ns: ['SYS.ECS,SYS.EVS,SYS.RDS']
```

### Grafana Alloy Configuration
```
prometheus.remote_write "mimir" {
  endpoint {
    url = "http://mimir:9009/api/v1/push"
  }
}

prometheus.scrape "cloudeye_exporter" {
  targets = [
    { __address__ = "<otc_cloudeye_exporter>:9098" },
  ]

  metrics_path    = "/metrics"
  scrape_interval = "10s"
  job_name        = "otc_cloudeye_exporter"

  params = {
    ns = ["SYS.ECS,SYS.VPC,SYS.ELB"],
  }

  forward_to = [prometheus.remote_write.mimir.receiver]
}
```

### Grafana Dashboard

Import metrics using the standard Prometheus data source. Key metric patterns:
- `otc_cloudeye_*` - All OTC Cloud Eye metrics
- Labels include: `project`, `namespace`, `resource_id`, `region`

## 🛠️ Troubleshooting

### Common Issues

**No metrics appearing**:
- Check log files for authentication errors
- Verify project names exist in the configured region
- Ensure network connectivity to OTC APIs

**Project validation failures**:
- Confirm project names are exact matches (case-sensitive)
- Verify credentials have permissions for the projects
- Check region configuration matches project locations

**SSL/TLS issues**:
- Set `ignore_ssl_verify: true` for development environments
- Ensure proper certificate configuration for production HTTPS

### Debug Mode

Enable debug logging in `logs.yml`:
```yaml
logging:
  - type: CONSOLE
    level: debug
    console:
      enabled: true
      encoder: "CONSOLE"
      time_format: "02.01.2006 15:04:05"
```

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## 📄 License

This project is licensed under the **Apache 2.0 License** - see the [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- Open Telekom Cloud (OTC) [API documentation and community support](https://docs.otc.t-systems.com)
- Powered by [Huawei Cloud SDK for Go v3](https://github.com/huaweicloud/huaweicloud-sdk-go-v3)
- Built with [Prometheus Go client library](https://github.com/prometheus/client_golang)