package cmd

import "testing"

func TestExtractMoleculeIDFromMail(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		expected string
	}{
		{
			name:     "attached_molecule field",
			body:     "Hello agent,\n\nattached_molecule: ms-abc123\n\nPlease work on this.",
			expected: "ms-abc123",
		},
		{
			name:     "molecule_id field",
			body:     "Work assignment:\nmolecule_id: mol-xyz789",
			expected: "mol-xyz789",
		},
		{
			name:     "molecule field",
			body:     "molecule: ms-task-42",
			expected: "ms-task-42",
		},
		{
			name:     "mol field",
			body:     "Quick task:\nmol: ms-quick\nDo this now.",
			expected: "ms-quick",
		},
		{
			name:     "no molecule field",
			body:     "This is just a regular message without any molecule.",
			expected: "",
		},
		{
			name:     "empty body",
			body:     "",
			expected: "",
		},
		{
			name:     "molecule with extra whitespace",
			body:     "attached_molecule:   ms-whitespace  \n\nmore text",
			expected: "ms-whitespace",
		},
		{
			name:     "multiple fields - first wins",
			body:     "attached_molecule: first\nmolecule: second",
			expected: "first",
		},
		{
			name:     "case insensitive line matching",
			body:     "Attached_Molecule: ms-case",
			expected: "ms-case",
		},
		{
			name:     "molecule in multiline context",
			body: `Subject: Work Assignment

This is your next task.

attached_molecule: ms-multiline

Please complete by EOD.

Thanks,
Overseer`,
			expected: "ms-multiline",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractMoleculeIDFromMail(tt.body)
			if result != tt.expected {
				t.Errorf("extractMoleculeIDFromMail(%q) = %q, want %q", tt.body, result, tt.expected)
			}
		})
	}
}
