package main

const (
	TemplTasks = `{{.Key}}. {{.Val}} by @{{.UserNameOwner}}{{if (.FreeTask)}}
/assign_{{.Key}}{{else if (.MyTask)}}{{if (.AssignBool)}}
assignee: я
/unassign_{{.Key}} /resolve_{{.Key}}{{else}}
/unassign_{{.Key}} /resolve_{{.Key}}{{end}}{{else}}{{if (.AssignBool)}}
assignee: @{{.UserNamePerform}}{{end}}{{end}}

`
	TemplNewTask = `Задача "{{.Task}}" создана, id={{.Id}}`
)
