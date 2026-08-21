package spec

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestSoftwareRefUnmarshalYAML covers both documented forms of a software ref
// decoded as a bare []SoftwareRef — not inside a Profile — because SoftwareRef
// is also the element type of Formation.Layers and LockFile.Defaults, and the
// forms have to mean the same thing in all of them.
func TestSoftwareRefUnmarshalYAML(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want SoftwareRef
	}{
		// The inline form. Every one of these was an unmarshal error before
		// SoftwareRef had an UnmarshalYAML method.
		{"inline name and version", "- python@3.13", SoftwareRef{Name: "python", Version: "3.13"}},
		{"inline three-part version", "- python@3.11.9", SoftwareRef{Name: "python", Version: "3.11.9"}},
		{"inline name only", "- cuda", SoftwareRef{Name: "cuda"}},
		{"inline formation", "- formation:bio-seq@2026.03", SoftwareRef{Formation: "bio-seq@2026.03"}},
		{"inline formation without version", "- formation:cuda-python-ml", SoftwareRef{Formation: "cuda-python-ml"}},
		{"inline quoted", `- "quarto@1.4"`, SoftwareRef{Name: "quarto", Version: "1.4"}},
		{"inline single quoted", `- 'git@2.43'`, SoftwareRef{Name: "git", Version: "2.43"}},

		// The mapping form, which worked before this change and must still.
		{"mapping name and version", "- {name: python, version: \"3.13\"}", SoftwareRef{Name: "python", Version: "3.13"}},
		{"mapping name only", "- {name: cuda}", SoftwareRef{Name: "cuda"}},
		{"mapping formation", "- {formation: bio-seq@2026.03}", SoftwareRef{Formation: "bio-seq@2026.03"}},
		{"mapping block style", "- name: quarto\n  version: \"1.4\"", SoftwareRef{Name: "quarto", Version: "1.4"}},

		// The mapping form has always accepted an inline ref in the name field.
		// normalizeSoftwareRefs did this for a profile's software list only;
		// UnmarshalYAML does it everywhere a SoftwareRef is decoded.
		{
			"mapping with inline formation in name",
			`- {name: "formation:cuda-python-ml@2024.03"}`,
			SoftwareRef{Formation: "cuda-python-ml@2024.03"},
		},
		{
			"mapping with inline name@version in name",
			`- {name: "cuda@12.3"}`,
			SoftwareRef{Name: "cuda", Version: "12.3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []SoftwareRef
			if err := yaml.Unmarshal([]byte(tt.yaml+"\n"), &got); err != nil {
				t.Fatalf("yaml.Unmarshal(%q) error: %v", tt.yaml, err)
			}
			if len(got) != 1 {
				t.Fatalf("yaml.Unmarshal(%q) gave %d refs, want 1", tt.yaml, len(got))
			}
			if got[0] != tt.want {
				t.Errorf("yaml.Unmarshal(%q) = %+v, want %+v", tt.yaml, got[0], tt.want)
			}
		})
	}
}

// TestSoftwareRefUnmarshalYAMLErrors covers the entries that must not decode,
// and asserts on the reason rather than merely on the presence of an error.
func TestSoftwareRefUnmarshalYAMLErrors(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{"inline name with a space", "- python 3.13", "invalid software ref"},
		{"inline name with a slash", "- cuda/12.3", "invalid software ref"},
		{"inline empty string", `- ""`, "empty software ref"},
		// Quoted, because bare "- formation:" is a mapping with a null value
		// rather than a scalar — see TestSoftwareRefNullOrEmptyEntries.
		{"inline formation with no name", `- "formation:"`, "empty formation name"},
		{"sequence is not a ref", "- [python, \"3.13\"]", "must be a string"},
		{"mapping with a non-string field", "- {name: [cuda]}", "cannot unmarshal"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []SoftwareRef
			err := yaml.Unmarshal([]byte(tt.yaml+"\n"), &got)
			if err == nil {
				t.Fatalf("yaml.Unmarshal(%q) succeeded, got %+v, want error", tt.yaml, got)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("yaml.Unmarshal(%q) error = %q, want it to contain %q", tt.yaml, err, tt.wantErr)
			}
		})
	}
}

// TestSoftwareRefUnmarshalYAMLReportsLine checks that a bad ref still names the
// line it is on. yaml.v3 returns an error from an unmarshaller verbatim instead
// of decorating it with a position, so the position is added by hand and would
// go missing silently if it were dropped.
func TestSoftwareRefUnmarshalYAMLReportsLine(t *testing.T) {
	input := []byte("- cuda\n- python\n- not a ref\n")
	var got []SoftwareRef
	err := yaml.Unmarshal(input, &got)
	if err == nil {
		t.Fatalf("yaml.Unmarshal succeeded, want error")
	}
	if !strings.Contains(err.Error(), "line 3") {
		t.Errorf("error = %q, want it to name line 3", err)
	}
}

// TestSoftwareRefNullOrEmptyEntries pins two behaviours that UnmarshalYAML does
// not control, both established by running them rather than by reading yaml.v3.
//
// A null entry is dropped from the sequence entirely: yaml.v3 stops at a null
// node before consulting an unmarshaller, and its decoder discards a sequence
// element whose unmarshal returned false instead of zeroing it. So a null branch
// inside UnmarshalYAML would be dead code, and the entry cannot be caught by
// Validate either — there is nothing left to validate. That is #79. This test
// asserts the defect as it stands so that #79 fails here when it is fixed,
// rather than leaving the hole undocumented.
//
// A bare "- formation:" is not a scalar at all — it is a mapping with a null
// value, so it takes the mapping branch and yields a zero ref, which a profile
// does reject.
func TestSoftwareRefNullOrEmptyEntries(t *testing.T) {
	t.Run("null entry is dropped (#79)", func(t *testing.T) {
		var got []SoftwareRef
		if err := yaml.Unmarshal([]byte("- {name: cuda}\n- ~\n"), &got); err != nil {
			t.Fatalf("yaml.Unmarshal error: %v", err)
		}
		if len(got) != 1 || got[0] != (SoftwareRef{Name: "cuda"}) {
			t.Fatalf("got %+v, want the null entry dropped leaving only {cuda} — see #79", got)
		}

		p, err := ParseProfileBytes([]byte("name: t\nbase:\n  os: al2023\nsoftware:\n  - python@3.13\n  - ~\n"))
		if err != nil {
			t.Fatalf("ParseProfileBytes() error: %v", err)
		}
		if len(p.Software) != 1 {
			t.Fatalf("len(Software) = %d, want 1 — two entries in, one out, no error (#79)", len(p.Software))
		}
	})

	t.Run("bare formation key is a mapping, not a scalar", func(t *testing.T) {
		var got []SoftwareRef
		if err := yaml.Unmarshal([]byte("- formation:\n"), &got); err != nil {
			t.Fatalf("yaml.Unmarshal error: %v", err)
		}
		if len(got) != 1 || got[0] != (SoftwareRef{}) {
			t.Fatalf("got %+v, want one zero-valued ref", got)
		}

		_, err := ParseProfileBytes([]byte("name: t\nbase:\n  os: al2023\nsoftware:\n  - formation:\n"))
		if err == nil {
			t.Fatal("ParseProfileBytes succeeded, want a validation error")
		}
		if !strings.Contains(err.Error(), "must have either name or formation set") {
			t.Errorf("error = %q, want it to report the ref has neither name nor formation", err)
		}
	})
}

// TestSoftwareRefMarshalYAML asserts the property MarshalYAML is written to
// hold: a ref is written inline only when the inline form reads back equal, and
// as a mapping otherwise. Every case must round-trip whichever form is chosen —
// including the cases that force the mapping fallback, which is why refs that
// cannot survive the inline form are in the table.
func TestSoftwareRefMarshalYAML(t *testing.T) {
	tests := []struct {
		name       string
		ref        SoftwareRef
		wantInline bool
	}{
		{"name and version", SoftwareRef{Name: "python", Version: "3.13"}, true},
		{"name only", SoftwareRef{Name: "cuda"}, true},
		{"formation", SoftwareRef{Formation: "bio-seq@2026.03"}, true},
		{"version holding a constraint", SoftwareRef{Name: "texlive", Version: ">=2024"}, true},

		// TrimSpace in ParseSoftwareRef eats the trailing space, so the inline
		// form would read back as "x" — not equal. Must fall back to a mapping.
		{"formation with a trailing space", SoftwareRef{Formation: "x "}, false},
		// String() gives "", which is not a ref at all.
		{"zero value", SoftwareRef{}, false},
		// Invalid: both set. String() would drop Name entirely.
		{"both name and formation", SoftwareRef{Name: "a", Formation: "b"}, false},
		// String() gives "@1.0", which parses to a ref with an empty name.
		{"version with no name", SoftwareRef{Version: "1.0"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := yaml.Marshal([]SoftwareRef{tt.ref})
			if err != nil {
				t.Fatalf("yaml.Marshal(%+v) error: %v", tt.ref, err)
			}

			// Inspect the emitted node kind rather than the formatting, so the
			// assertion is about the form chosen and not about yaml's layout.
			var seq []yaml.Node
			if err := yaml.Unmarshal(data, &seq); err != nil {
				t.Fatalf("re-reading %q as nodes: %v", data, err)
			}
			if len(seq) != 1 {
				t.Fatalf("marshalled to %d entries, want 1: %q", len(seq), data)
			}
			gotInline := seq[0].Kind == yaml.ScalarNode
			if gotInline != tt.wantInline {
				t.Errorf("yaml.Marshal(%+v) = %q, inline=%v, want inline=%v",
					tt.ref, data, gotInline, tt.wantInline)
			}

			var back []SoftwareRef
			if err := yaml.Unmarshal(data, &back); err != nil {
				t.Fatalf("round-trip of %+v through %q failed to re-read: %v", tt.ref, data, err)
			}
			if len(back) != 1 || back[0] != tt.ref {
				t.Errorf("round-trip of %+v through %q gave %+v", tt.ref, data, back)
			}
		})
	}
}

// TestProfileRoundTripsInlineForm checks the whole-document path: a profile
// written in the documented inline form parses, and re-marshalling it produces a
// document that parses to the same refs.
func TestProfileRoundTripsInlineForm(t *testing.T) {
	input := []byte(`name: inline
base:
  os: al2023
software:
  - formation:bio-seq@2026.03
  - quarto@1.4
  - git@2.43
defaults:
  - quarto@1.4
`)
	p, err := ParseProfileBytes(input)
	if err != nil {
		t.Fatalf("ParseProfileBytes() error: %v", err)
	}

	want := []SoftwareRef{
		{Formation: "bio-seq@2026.03"},
		{Name: "quarto", Version: "1.4"},
		{Name: "git", Version: "2.43"},
	}
	if len(p.Software) != len(want) {
		t.Fatalf("len(Software) = %d, want %d", len(p.Software), len(want))
	}
	for i := range want {
		if p.Software[i] != want[i] {
			t.Errorf("Software[%d] = %+v, want %+v", i, p.Software[i], want[i])
		}
	}
	// Defaults is a []SoftwareRef too, and normalizeSoftwareRefs never walked
	// it. The inline form has to work here as well.
	if len(p.Defaults) != 1 || p.Defaults[0] != (SoftwareRef{Name: "quarto", Version: "1.4"}) {
		t.Errorf("Defaults = %+v, want one {quarto 1.4} ref", p.Defaults)
	}

	data, err := MarshalProfile(p)
	if err != nil {
		t.Fatalf("MarshalProfile() error: %v", err)
	}
	p2, err := ParseProfileBytes(data)
	if err != nil {
		t.Fatalf("ParseProfileBytes() after marshal error: %v\nmarshalled:\n%s", err, data)
	}
	for i := range p.Software {
		if p.Software[i] != p2.Software[i] {
			t.Errorf("round-trip Software[%d]: %+v != %+v", i, p.Software[i], p2.Software[i])
		}
	}
}
