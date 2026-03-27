{{- define "flowpulse.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "flowpulse.fullname" -}}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{- define "flowpulse.namespace" -}}
{{- default .Release.Namespace .Values.namespaceOverride }}
{{- end }}

{{- define "flowpulse.labels" -}}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: flowpulse
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}

{{- define "flowpulse.selectorLabels" -}}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "flowpulse.image" -}}
{{- $registry := .global.imageRegistry | default "" -}}
{{- $repo := .image.repository -}}
{{- $tag := .image.tag | default "latest" -}}
{{- if $registry -}}
{{- printf "%s/%s:%s" $registry $repo $tag -}}
{{- else -}}
{{- printf "%s:%s" $repo $tag -}}
{{- end -}}
{{- end }}

{{- define "flowpulse.secretName" -}}
{{- printf "%s-secrets" (include "flowpulse.fullname" .) }}
{{- end }}

{{- define "flowpulse.clickhouse.dsn" -}}
{{- if .Values.clickhouse.dsn -}}
{{- .Values.clickhouse.dsn -}}
{{- else if .Values.clickhouse.enabled -}}
{{- printf "clickhouse://%s-clickhouse:9000/%s" (include "flowpulse.fullname" .) .Values.clickhouse.database -}}
{{- else -}}
clickhouse://localhost:9000/flowpulse
{{- end -}}
{{- end }}

{{- define "flowpulse.nats.url" -}}
{{- if .Values.nats.url -}}
{{- .Values.nats.url -}}
{{- else if .Values.nats.enabled -}}
{{- printf "nats://%s-nats:4222" (include "flowpulse.fullname" .) -}}
{{- else -}}
nats://localhost:4222
{{- end -}}
{{- end }}

{{- define "flowpulse.redis.addr" -}}
{{- if .Values.redis.addr -}}
{{- .Values.redis.addr -}}
{{- else if .Values.redis.enabled -}}
{{- printf "%s-redis:6379" (include "flowpulse.fullname" .) -}}
{{- else -}}
localhost:6379
{{- end -}}
{{- end }}
