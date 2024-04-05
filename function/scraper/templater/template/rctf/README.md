---
title: "{{.Name}} | Challenge"
---

{{.Name}} - {{.Category}}
===

## Description

{{.Description}}

{{if .Files}}## Files
{{range $_, $v := .Files}}- [{{$v.FileName}}](attachment/{{$v.FileName}})

{{end}}{{end}}
