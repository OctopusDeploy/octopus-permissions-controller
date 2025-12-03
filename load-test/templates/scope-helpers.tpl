{{- define "selectFromFile" -}}
{{- $list := splitList "\n" (ReadFile (printf "scope-values/%s.txt" .file)) | compact -}}
{{- $idx := mod .seed (len $list) -}}
{{- index $list $idx -}}
{{- end -}}

{{- define "firstFromFile" -}}
{{- $list := splitList "\n" (ReadFile (printf "scope-values/%s.txt" .file)) | compact -}}
{{- index $list 0 -}}
{{- end -}}
