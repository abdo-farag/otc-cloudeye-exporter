{{/*
Expand the name of the chart.
*/}}
{{- define "otc-cloudeye-exporter.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "otc-cloudeye-exporter.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "otc-cloudeye-exporter.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "otc-cloudeye-exporter.labels" -}}
helm.sh/chart: {{ include "otc-cloudeye-exporter.chart" . }}
{{ include "otc-cloudeye-exporter.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "otc-cloudeye-exporter.selectorLabels" -}}
app.kubernetes.io/name: {{ include "otc-cloudeye-exporter.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "otc-cloudeye-exporter.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "otc-cloudeye-exporter.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the name of the secret to use for OTC credentials
*/}}
{{- define "otc-cloudeye-exporter.secretName" -}}
{{- if .Values.otcCredentials.existingSecret }}
{{- .Values.otcCredentials.existingSecret }}
{{- else }}
{{- printf "%s-credentials" (include "otc-cloudeye-exporter.fullname" .) }}
{{- end }}
{{- end }}

{{/*
Create the name of the configmap to use for clouds.yml
*/}}
{{- define "otc-cloudeye-exporter.cloudsConfigMapName" -}}
{{- if .Values.cloudsConfig.existingConfigMap }}
{{- .Values.cloudsConfig.existingConfigMap }}
{{- else }}
{{- printf "%s-clouds" (include "otc-cloudeye-exporter.fullname" .) }}
{{- end }}
{{- end }}