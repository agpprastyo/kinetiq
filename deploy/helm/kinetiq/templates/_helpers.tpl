{{- define "kinetiq.labels" -}}
app.kubernetes.io/name: {{ .Chart.Name }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}

{{- define "kinetiq.selector" -}}
app.kubernetes.io/instance: {{ .root.Release.Name }}
app.kubernetes.io/component: {{ .component }}
{{- end }}

{{- define "kinetiq.image" -}}
{{ .root.Values.image.registry }}/{{ .name }}:{{ .root.Values.image.tag | default .root.Chart.AppVersion }}
{{- end }}

{{- define "kinetiq.antiAffinity" -}}
affinity:
  podAntiAffinity:
    preferredDuringSchedulingIgnoredDuringExecution:
      - weight: 100
        podAffinityTerm:
          topologyKey: kubernetes.io/hostname
          labelSelector:
            matchLabels:
              {{- include "kinetiq.selector" . | nindent 14 }}
{{- end }}

{{/* Shared env for api + worker (Go backend). */}}
{{- define "kinetiq.backendEnv" -}}
envFrom:
  - secretRef:
      name: {{ .Values.existingSecret }}
env:
  {{- range $k, $v := .Values.config }}
  - name: {{ $k }}
    value: {{ tpl $v $ | quote }}
  {{- end }}
  {{- with .Values.redis.sentinelAddrs }}
  - name: REDIS_SENTINEL_ADDRS
    value: {{ . | quote }}
  - name: REDIS_SENTINEL_MASTER
    value: {{ required "redis.sentinelMaster is required when redis.sentinelAddrs is set" $.Values.redis.sentinelMaster | quote }}
  {{- end }}
{{- end }}

{{- define "kinetiq.pdb" -}}
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: {{ .root.Release.Name }}-{{ .component }}
  labels: {{- include "kinetiq.labels" .root | nindent 4 }}
spec:
  minAvailable: {{ .values.pdb.minAvailable }}
  selector:
    matchLabels: {{- include "kinetiq.selector" . | nindent 6 }}
{{- end }}
