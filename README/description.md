Marshal [golangci-lint v2](https://golangci-lint.run)'s `config.Config` to JSON.

```go
import (
	"log"

	"os"

	golangcijson "github.com/MarkRosemaker/golangci-json"
	"github.com/golangci/golangci-lint/v2/pkg/config"
)

func main() {
	var cfg = config.Config{
		Version: "2",
		Linters: config.Linters{
			Default: config.GroupNone,
			Enable:  []string{"tagalign"},
			Settings: config.LintersSettings{
				TagAlign: config.TagAlignSettings{Align: true, Sort: true},
			},
		},
	}

	if err := golangcijson.MarshalWrite(os.Stdout, cfg); err != nil {
		log.Fatal(err)
	}
}
```

```json
{
  "version": "2",
  "linters": {
    "default": "none",
    "enable": [
      "tagalign"
    ],
    "settings": {
      "tagalign": {
        "align": true,
        "sort": true
      }
    }
  }
}
```
