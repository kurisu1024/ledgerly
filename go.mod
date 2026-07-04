module github.com/kurisu1024/ledgerly

go 1.25.5

require (
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/jackc/pgx/v5 v5.7.6
	github.com/spf13/cobra v1.10.2
	go.uber.org/zap v1.27.1
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	github.com/stretchr/testify v1.11.1 // indirect
	go.uber.org/multierr v1.10.0 // indirect
)

require (
	github.com/google/uuid v1.6.0
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	golang.org/x/crypto v0.40.0 // indirect
	golang.org/x/sync v0.16.0 // indirect
	golang.org/x/text v0.27.0 // indirect
)

// sdk/go is a separate Go module (github.com/kurisu1024/ledgerly/sdk/go,
// issue #26) with zero dependency on this one, joined for local dev via
// go.work. make test-go runs GOWORK=off, so the root module needs its own
// require + replace to resolve the import service/ uses for dogfood
// wiring (issue #27) — see sdk/go/README.md.
require github.com/kurisu1024/ledgerly/sdk/go v0.0.0

replace github.com/kurisu1024/ledgerly/sdk/go => ./sdk/go
