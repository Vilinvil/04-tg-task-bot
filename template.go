package main

const (
	TemplTasks = `{{.Key}}. {{.Val}} by @{{.UserNameOwner}}{{if (.FreeTask)}}
/assign_{{.Key}}{{else}}{{if (.MyTask)}}
assignee: я
/unassign_{{.Key}} /resolve_{{.Key}}{{else}}
assignee: @{{.UserNamePerform}}{{end}}{{end}}

`
	TemplNewTask = `Задача "{{.Task}}" создана, id={{.Id}}`
)
