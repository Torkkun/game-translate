package translate

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Speaker は話者プロフィール。
type Speaker struct {
	Note        string `yaml:"note"`         // 性別・立場・口調など (モデルへの文脈)
	FirstPerson string `yaml:"first_person"` // 一人称 (例 "俺", "私")。違う一人称が出たら警告する
}

// LoadSpeakers は話者プロフィールの YAML (characters: {名前: {note, first_person}}) を読む。
// path が空なら nil。
func LoadSpeakers(path string) (map[string]Speaker, error) {
	if path == "" {
		return nil, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var v struct {
		Characters map[string]Speaker `yaml:"characters"`
	}
	if err := yaml.Unmarshal(b, &v); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return v.Characters, nil
}

// 一人称のうち、他の語の一部になりにくいもの。「私」は「私立」「私物」などと紛れるので判定に使わない。
var distinctFirstPersons = []string{"俺", "僕", "あたし"} // 「わし」は「出くわし」「そわそわし」と紛れるので使わない

// checkFirstPerson は話者の一人称と違う一人称が訳文に出ていないかを調べる。
func checkFirstPerson(target string, sp Speaker) []string {
	if sp.FirstPerson == "" {
		return nil
	}
	var w []string
	for _, fp := range distinctFirstPersons {
		if fp != sp.FirstPerson && strings.Contains(target, fp) {
			w = append(w, fmt.Sprintf("一人称が違う (「%s」、この話者は「%s」)", fp, sp.FirstPerson))
		}
	}
	return w
}
