package examples

import (
	"log"
	"os"

	golangcijson "github.com/MarkRosemaker/golangci-json"
	"github.com/golangci/golangci-lint/v2/pkg/config"
)

func Example_json() {
	cfg := config.Config{
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
	// Output:
	// {
	//   "version": "2",
	//   "linters": {
	//     "default": "none",
	//     "enable": [
	//       "tagalign"
	//     ],
	//     "settings": {
	//       "tagalign": {
	//         "align": true,
	//         "sort": true
	//       }
	//     }
	//   }
	// }
}
