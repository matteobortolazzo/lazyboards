package main

// NewBoard returns a Board pre-populated with 5 Kanban columns and fake cards.
func NewBoard() Board {
	return Board{
		Columns: []Column{
			{
				Title: "New",
				Cards: []Card{
					{Number: 1, Title: "Setup CI", Label: "infra"},
					{Number: 2, Title: "Data model", Label: "design"},
					{Number: 3, Title: "Add README", Label: "docs"},
				},
			},
			{
				Title: "Refined",
				Cards: []Card{
					{Number: 4, Title: "User auth", Label: "feature"},
					{Number: 5, Title: "API routes", Label: "backend"},
					{Number: 6, Title: "Error types", Label: "backend"},
					{Number: 7, Title: "DB migrate", Label: "infra"},
				},
			},
			{
				Title: "Implementing",
				Cards: []Card{
					{Number: 8, Title: "Board view", Label: "feature"},
					{Number: 9, Title: "Key binds", Label: "feature"},
					{Number: 10, Title: "Col nav", Label: "feature"},
					{Number: 11, Title: "Lipgloss", Label: "ui"},
					{Number: 12, Title: "Config", Label: "feature"},
				},
			},
			{
				Title: "PR Ready",
				Cards: []Card{
					{Number: 13, Title: "Fix clamp", Label: "bug"},
					{Number: 14, Title: "Refactor", Label: "chore"},
					{Number: 15, Title: "Help bar", Label: "ui"},
				},
			},
			{
				Title: "Done",
				Cards: []Card{
					{Number: 16, Title: "Go module", Label: "infra"},
					{Number: 17, Title: "Scaffold", Label: "feature"},
					{Number: 18, Title: "Fake data", Label: "feature"},
					{Number: 19, Title: "Tests", Label: "test"},
				},
			},
		},
	}
}
