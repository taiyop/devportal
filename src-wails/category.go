package main

import (
	"regexp"
	"strings"

	"github.com/google/uuid"
)

var categoryColorPattern = regexp.MustCompile(`^#[0-9a-f]{6}$`)

func categoryExists(inner *Inner, id string) bool {
	for _, cat := range inner.file.Categories {
		if cat.ID == id {
			return true
		}
	}
	return false
}

func (p *Portal) Categories() []CategoryEntry {
	inner := p.lock()
	defer p.unlock()
	out := make([]CategoryEntry, len(inner.file.Categories))
	copy(out, inner.file.Categories)
	return out
}

func (p *Portal) UpsertCategory(id string, name string, color string) (CategoryEntry, error) {
	inner := p.lock()
	defer p.unlock()
	name = strings.TrimSpace(name)
	if name == "" {
		return CategoryEntry{}, errString("カテゴリ名を入力してください")
	}
	color = strings.ToLower(strings.TrimSpace(color))
	if color != "" && !categoryColorPattern.MatchString(color) {
		return CategoryEntry{}, errString("カテゴリの色は #rrggbb 形式で指定してください")
	}
	for _, cat := range inner.file.Categories {
		if cat.ID != id && strings.EqualFold(cat.Name, name) {
			return CategoryEntry{}, errString("同じ名前のカテゴリがあります: " + name)
		}
	}
	entry := CategoryEntry{ID: strings.TrimSpace(id), Name: name, Color: color}
	if entry.ID == "" {
		entry.ID = uuid.NewString()
	}
	replaced := false
	for i := range inner.file.Categories {
		if inner.file.Categories[i].ID == entry.ID {
			inner.file.Categories[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		inner.file.Categories = append(inner.file.Categories, entry)
	}
	if err := saveConfig(inner.configPath, inner.file); err != nil {
		return CategoryEntry{}, err
	}
	return entry, nil
}

func (p *Portal) DeleteCategory(id string) error {
	inner := p.lock()
	before := len(inner.file.Categories)
	next := inner.file.Categories[:0]
	for _, cat := range inner.file.Categories {
		if cat.ID != id {
			next = append(next, cat)
		}
	}
	inner.file.Categories = next
	if len(inner.file.Categories) == before {
		p.unlock()
		return errString("カテゴリが見つかりません")
	}
	affected := []string{}
	for i := range inner.file.Apps {
		if inner.file.Apps[i].CategoryID == id {
			inner.file.Apps[i].CategoryID = ""
			affected = append(affected, inner.file.Apps[i].ID)
		}
	}
	err := saveConfig(inner.configPath, inner.file)
	p.unlock()
	if err != nil {
		return err
	}
	for _, appID := range affected {
		p.emitView(appID)
	}
	return nil
}

func (p *Portal) ReorderCategories(ids []string) ([]CategoryEntry, error) {
	inner := p.lock()
	defer p.unlock()
	byID := make(map[string]CategoryEntry, len(inner.file.Categories))
	for _, cat := range inner.file.Categories {
		byID[cat.ID] = cat
	}
	next := make([]CategoryEntry, 0, len(inner.file.Categories))
	seen := make(map[string]bool, len(ids))
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if id == "" || seen[id] {
			continue
		}
		if cat, ok := byID[id]; ok {
			next = append(next, cat)
			seen[id] = true
		}
	}
	for _, cat := range inner.file.Categories {
		if !seen[cat.ID] {
			next = append(next, cat)
		}
	}
	inner.file.Categories = next
	if err := saveConfig(inner.configPath, inner.file); err != nil {
		return nil, err
	}
	return next, nil
}
