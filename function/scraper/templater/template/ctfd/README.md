---

title: "{{.Name}} | Challenge"

---

{{.Name}} - {{.Category}}
===

## Description

{{.Description}}
{{if .Connection_Info}}Connection Info:

:::info
{{.Connection_Info}}
:::

{{end}}{{if .Files}}## Files
{{range $_, $v := .Files}}- [{{$v.FileName}}](attachment/{{$v.FileName}})

{{end}}{{end}}