package stack

import "fmt"

type Environment struct {
	Name                      string
	WorkDir                   string
	ComposeFile               string
	RequiredConfigFiles       []string
	DevInfrastructureServices []string
}

func Resolve(name string) (Environment, error) {
	switch name {
	case "", "dev":
		return Environment{
			Name:        "dev",
			WorkDir:     "dev",
			ComposeFile: "dev/docker-compose.yaml",
			RequiredConfigFiles: []string{
				"dev/config.yaml",
				"dev/config.rescanner.yaml",
				"dev/config.indexer.yaml",
			},
			DevInfrastructureServices: []string{
				"migrate",
				"apex",
				"rescanner",
				"postgres",
				"redis",
				"mongo",
				"nats-server",
				"consul",
				"multichain-indexer",
				"fystack-ui-community",
			},
		}, nil
	case "prod":
		return Environment{
			Name:        "prod",
			WorkDir:     "prod",
			ComposeFile: "prod/docker-compose.yaml",
		}, nil
	default:
		return Environment{}, fmt.Errorf("unknown environment %q; use dev or prod", name)
	}
}
