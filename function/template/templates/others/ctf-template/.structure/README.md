# {{ .Name }}

> Please check the [writeups](./writeups/) for adding writeups to this repository, and refer to the [solver](./solver/) if an author solver exists.

**Author:** {{ .Author }}
{{ if .Provide }}
**Attachment:** [{{ .Provide }}]({{ .Provide }})
{{ end }}

## Description
{{ .Description }}