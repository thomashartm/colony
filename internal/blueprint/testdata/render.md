+++
schema = 1
name = "fixture"
vars = ["constraints", "missing"]
future_key = "ignored"
+++
{{.Repo}}: {{.Name}} (#{{.Ticket}})
{{.Branch}} -> {{.Base}}
{{.Worktree}}
{{.Crew.Title}} | {{.Crew.URL}} | {{.Crew.Kind}}
Constraints: {{.Vars.constraints}}
Unset: [{{.Vars.missing}}] [{{.Vars.undeclared}}]
{{if .Ticket}}Ticket present{{else}}No ticket{{end}}
Literal: <no value> $(touch SHOULD_NOT_RUN) `printf x` "quotes"
