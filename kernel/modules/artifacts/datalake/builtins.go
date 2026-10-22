// SPDX-License-Identifier: MIT

package datalake

// builtins.go defines the Personal Data Lake's out-of-the-box collections (M835)
// — the everyday "apps" the owner asked for (expenses, calendar, tasks, notes,
// habits, bookmarks, contacts). They are seeded at boot via SeedBuiltins and
// marked System so they're always present (the Web UI renders each with its own
// bespoke view named by Schema.View). Records inside are fully agent/user
// managed; only the collection definitions are fixed.

// BuiltinSchemas returns the definitions of the built-in collections. Pure data
// (no I/O) so it's trivially testable and reusable. Field Types are UI/agent
// hints: text | number | money | date | bool | url | tags | note.
func BuiltinSchemas() []Schema {
	return []Schema{
		{
			Name: "expenses", Title: "Expenses", Icon: "wallet", View: "expense",
			Desc:    "Track spending — amount, category, date.",
			Builtin: true, System: true,
			Fields: []Field{
				{Name: "date", Type: "date", Label: "Date"},
				{Name: "item", Type: "text", Label: "Item"},
				{Name: "amount", Type: "money", Label: "Amount"},
				{Name: "category", Type: "text", Label: "Category"},
				{Name: "note", Type: "note", Label: "Note"},
			},
		},
		{
			Name: "calendar", Title: "Calendar", Icon: "calendar", View: "calendar",
			Desc:    "Events and appointments.",
			Builtin: true, System: true,
			Fields: []Field{
				{Name: "title", Type: "text", Label: "Title"},
				{Name: "date", Type: "date", Label: "Date"},
				{Name: "time", Type: "text", Label: "Time"},
				{Name: "location", Type: "text", Label: "Location"},
				{Name: "note", Type: "note", Label: "Note"},
			},
		},
		{
			Name: "tasks", Title: "Tasks", Icon: "list-todo", View: "tasks",
			Desc:    "To-dos with status, due date, priority.",
			Builtin: true, System: true,
			Fields: []Field{
				{Name: "title", Type: "text", Label: "Title"},
				{Name: "done", Type: "bool", Label: "Done"},
				{Name: "due", Type: "date", Label: "Due"},
				{Name: "priority", Type: "text", Label: "Priority"},
				{Name: "note", Type: "note", Label: "Note"},
			},
		},
		{
			Name: "notes", Title: "Notes", Icon: "sticky-note", View: "notes",
			Desc:    "Free-form notes.",
			Builtin: true, System: true,
			Fields: []Field{
				{Name: "title", Type: "text", Label: "Title"},
				{Name: "body", Type: "note", Label: "Body"},
				{Name: "tags", Type: "tags", Label: "Tags"},
			},
		},
		{
			Name: "habits", Title: "Habits", Icon: "repeat", View: "habits",
			Desc:    "Habit tracking with streaks.",
			Builtin: true, System: true,
			Fields: []Field{
				{Name: "name", Type: "text", Label: "Habit"},
				{Name: "streak", Type: "number", Label: "Streak"},
				{Name: "last", Type: "date", Label: "Last done"},
				{Name: "cadence", Type: "text", Label: "Cadence"},
			},
		},
		{
			Name: "bookmarks", Title: "Bookmarks", Icon: "bookmark", View: "bookmarks",
			Desc:    "Saved links.",
			Builtin: true, System: true,
			Fields: []Field{
				{Name: "title", Type: "text", Label: "Title"},
				{Name: "url", Type: "url", Label: "URL"},
				{Name: "tags", Type: "tags", Label: "Tags"},
				{Name: "note", Type: "note", Label: "Note"},
			},
		},
		{
			Name: "contacts", Title: "Contacts", Icon: "contact", View: "contacts",
			Desc:    "People and their details.",
			Builtin: true, System: true,
			Fields: []Field{
				{Name: "name", Type: "text", Label: "Name"},
				{Name: "email", Type: "text", Label: "Email"},
				{Name: "phone", Type: "text", Label: "Phone"},
				{Name: "company", Type: "text", Label: "Company"},
				{Name: "note", Type: "note", Label: "Note"},
			},
		},
	}
}

// SeedBuiltins ensures every built-in collection exists, leaving any that are
// already present (and their data) untouched. Returns the names that were newly
// created this call. Idempotent — safe to run on every boot.
func (l *Lake) SeedBuiltins(actor string) ([]string, error) {
	var created []string
	for _, sc := range BuiltinSchemas() {
		_, isNew, err := l.EnsureCollection(sc, actor)
		if err != nil {
			return created, err
		}
		if isNew {
			created = append(created, sc.Name)
		}
	}
	return created, nil
}
