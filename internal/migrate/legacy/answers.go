package legacy

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Answer kinds an operator can supply in answers.csv (kind,legacy_id,answer).
const (
	AnswerTariffClass         = "tariff_class"          // building hex → lv|mv/<group>/<term>/<supply>
	AnswerTariffTemplateClass = "tariff_template_class" // template hex → the same form
	AnswerISOBuilding         = "iso_building"          // user hex → building hex
)

var answerKinds = map[string]bool{AnswerTariffClass: true, AnswerTariffTemplateClass: true, AnswerISOBuilding: true}

// ReadAnswers reads an operator's answers.csv; unknown kinds fail loudly.
func ReadAnswers(path string) (map[string]map[string]string, error) {
	f, err := os.Open(path) //nolint:gosec // an operator-supplied file
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	records, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, err
	}
	out := map[string]map[string]string{}
	for i, r := range records {
		if i == 0 && len(r) > 0 && r[0] == "kind" {
			continue
		}
		if len(r) < 3 || strings.TrimSpace(r[2]) == "" {
			continue
		}
		kind := strings.TrimSpace(r[0])
		if !answerKinds[kind] {
			return nil, fmt.Errorf("%s line %d: unknown answer kind %q", path, i+1, kind)
		}
		if out[kind] == nil {
			out[kind] = map[string]string{}
		}
		out[kind][strings.TrimSpace(r[1])] = strings.TrimSpace(r[2])
	}
	return out, nil
}

// manualItem is one fact the transform could not determine: it is listed in
// manual_<kind>.csv for an answer, never guessed.
type manualItem struct{ legacyID, detail string }

func (t *transformer) ask(kind, legacyID, detail string) {
	for _, m := range t.asked[kind] {
		if m.legacyID == legacyID {
			return
		}
	}
	t.asked[kind] = append(t.asked[kind], manualItem{legacyID, detail})
}

func (t *transformer) answer(kind, legacyID string) string { return t.opt.Answers[kind][legacyID] }

func (t *transformer) writeAsked(dir string) error {
	kinds := make([]string, 0, len(t.asked))
	for k := range t.asked {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	for _, kind := range kinds {
		items := t.asked[kind]
		sort.Slice(items, func(i, j int) bool { return items[i].legacyID < items[j].legacyID })
		f, err := os.Create(filepath.Join(dir, "manual_"+kind+".csv")) //nolint:gosec // staging directory
		if err != nil {
			return err
		}
		cw := csv.NewWriter(f)
		_ = cw.Write([]string{"kind", "legacy_id", "answer", "detail"})
		for _, m := range items {
			_ = cw.Write([]string{kind, m.legacyID, "", m.detail})
		}
		cw.Flush()
		if err := errors.Join(cw.Error(), f.Close()); err != nil {
			return err
		}
	}
	return nil
}

// note records a changed-on-purpose fact (not a rejection) for review and for
// reconcile's attribution (F14c): notes.ndjson + a count in the summary.
func (t *transformer) note(collection, legacyID, what string, detail any) error {
	t.notes[what]++
	b, err := json.Marshal(map[string]any{"collection": collection, "legacy_id": legacyID, "note": what, "detail": detail})
	if err != nil {
		return err
	}
	if _, err := t.notesOut.Write(append(b, '\n')); err != nil {
		return err
	}
	return nil
}
