package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/Torkkun/game-translate/internal/gameconf"
	"github.com/Torkkun/game-translate/internal/install"
)

// cmdInstall は dist/<game> の Mod 一式をゲームフォルダに入れる。
func cmdInstall(args []string) error {
	flags := flag.NewFlagSet("install", flag.ExitOnError)
	game := flags.String("game", "", "ゲーム名 (games/<game>/game.yaml)")
	dir := flags.String("dir", "", "ゲームフォルダ (省略時は game.yaml の install_dir)")
	dist := flags.String("dist", "", "Mod 一式 (省略時 dist/<game>)")
	flags.Parse(args)
	if *game == "" {
		flags.Usage()
		return fmt.Errorf("-game は必須です")
	}
	c, err := gameconf.Load(*game)
	if err != nil {
		return err
	}
	if *dir == "" {
		*dir = c.InstallDir
	}
	if *dist == "" {
		*dist = "dist/" + *game
	}
	res, err := install.Install(*game, *dist, *dir, c.Exe, "work/"+*game+"/backup")
	if res != nil {
		log.Printf("コピー %d / 変更なし %d / 上書き前に退避 %d", res.Copied, res.Unchanged, res.BackedUp)
	}
	if err != nil {
		return err
	}
	log.Printf("%s に Mod を入れました (記録: %s)", *dir, install.ManifestName)
	return nil
}

// cmdUninstall は gtl install で入れた Mod をゲームフォルダから外す。
func cmdUninstall(args []string) error {
	flags := flag.NewFlagSet("uninstall", flag.ExitOnError)
	game := flags.String("game", "", "ゲーム名 (games/<game>/game.yaml)")
	dir := flags.String("dir", "", "ゲームフォルダ (省略時は game.yaml の install_dir)")
	flags.Parse(args)
	if *game == "" {
		flags.Usage()
		return fmt.Errorf("-game は必須です")
	}
	c, err := gameconf.Load(*game)
	if err != nil {
		return err
	}
	if *dir == "" {
		*dir = c.InstallDir
	}
	// プレイ中にできた記録や設定 (game.yaml の preserve に書いたファイル名) は消す前に退避する
	res, err := install.Uninstall(*dir, "work/"+*game+"/preserved", c.Preserve)
	if res != nil {
		log.Printf("削除 %d / 元のファイルを復元 %d", res.Removed, res.Restored)
		for _, p := range res.Preserved {
			log.Printf("退避: %s", p)
		}
	}
	if err != nil {
		return err
	}
	log.Printf("%s から Mod を外しました", *dir)
	return nil
}
