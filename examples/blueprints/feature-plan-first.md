+++
schema = 1
name = "feature-plan-first"
description = "Plan, wait for approval, then implement"
agent = "claude"
args = ["--permission-mode", "plan"]
vars = ["constraints"]
+++
You are working on {{.Name}} in {{.Repo}}.
{{if .Ticket}}Ticket: #{{.Ticket}}
{{end}}Branch: {{.Branch}} (base {{.Base}})
Worktree: {{.Worktree}}
{{if .Crew.Title}}Crew: {{.Crew.Title}} {{.Crew.URL}}
{{end}}
Constraints: {{.Vars.constraints}}
Start with a plan. Do not write code until I approve it.
