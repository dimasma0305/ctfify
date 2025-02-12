# {{ .name }}

> Please check the [writeups](./writeups/) for adding writeups to this repository, and refer to the [solver](./solver/) if an author solver exists.

**Author:** {{ .author }}
{{ if .provide }}
**Attachment:** [{{ .provide }}]({{ .provide }})
{{ end }}

## Description
{{ .description }}