/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"github.com/dimasma0305/ctfify/function/addons"
	"github.com/dimasma0305/ctfify/function/log"
	"github.com/lqqyt2423/go-mitmproxy/proxy"
	"github.com/lqqyt2423/go-mitmproxy/web"
	"github.com/spf13/cobra"
)

var proxyFlag struct {
	proxyAddr            string
	web                  bool
	webAddr              string
	reflectedXSS         bool
	crossOriginChecker   bool
	requestMapper        bool
	requestMapperSaveDir string
	requestMapperRegex   string
}

// proxyCmd represents the proxy command
var proxyCmd = &cobra.Command{
	Use:   "proxy",
	Short: "proxy",
	Long:  `proxy`,
	Run: func(cmd *cobra.Command, args []string) {
		opts := &proxy.Options{
			Addr:              proxyFlag.proxyAddr,
			StreamLargeBodies: 1024 * 1024 * 5,
			SslInsecure:       true,
		}
		p, err := proxy.NewProxy(opts)
		if err != nil {
			log.Fatal(err)
		}

		if proxyFlag.web {
			p.AddAddon(web.NewWebAddon(proxyFlag.webAddr))
		}

		if proxyFlag.reflectedXSS {
			p.AddAddon(&addons.Reflected{})
		}

		if proxyFlag.requestMapper {
			requestMapper, err := addons.NewRequestMapper(proxyFlag.requestMapperSaveDir, proxyFlag.requestMapperRegex)
			if err != nil {
				log.Fatal(err)
			}
			p.AddAddon(requestMapper)
		}
		if err := p.Start(); err != nil {
			log.Fatal(err)
		}
	},
}

func init() {
	rootCmd.AddCommand(proxyCmd)
	proxyCmd.Flags().StringVarP(&proxyFlag.proxyAddr, "addr", "p", ":8080", "define address to use")
	proxyCmd.Flags().BoolVar(&proxyFlag.web, "web", false, "activate web interface")
	proxyCmd.Flags().StringVar(&proxyFlag.webAddr, "web-addr", ":8000", "set web interface address")
	proxyCmd.Flags().BoolVar(&proxyFlag.reflectedXSS, "reflected-xss", false, "activate reflected xss addon")
	proxyCmd.Flags().BoolVar(&proxyFlag.crossOriginChecker, "cross-origin-checker", false, "activate cross origin checker")
	proxyCmd.Flags().BoolVar(&proxyFlag.requestMapper, "request-mapper", false, "activate request mapper")
	proxyCmd.Flags().StringVar(&proxyFlag.requestMapperSaveDir, "request-mapper-dir", ".", "request mapper save dir")
	proxyCmd.Flags().StringVar(&proxyFlag.requestMapperRegex, "request-mapper-regex", "^.*$", "request mapper regex")
}
