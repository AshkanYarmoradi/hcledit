package command

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"strings"
	"text/template"

	"github.com/spf13/cobra"
	yaml "gopkg.in/yaml.v2"

	"go.mercari.io/hcledit"
)

type ReadOptions struct {
	OutputFormat string
	Fallback     bool
}

func NewCmdRead() *cobra.Command {
	opts := &ReadOptions{}
	cmd := &cobra.Command{
		Use:   "read <query> <file>",
		Short: "Read a value",
		Long:  `Runs an address query on a hcl file and prints the result`,
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			result, err := runRead(opts, args)
			if err != nil {
				return err
			}

			fmt.Print(result)
			return nil
		},
	}

	cmd.Flags().StringVarP(&opts.OutputFormat, "output-format", "o", "go-template='{{.Value}}'", "format to print the value as")
	cmd.Flags().BoolVar(&opts.Fallback, "fallback", false, "falls back to reading the raw value if it cannot be evaluated")

	return cmd
}

func runRead(opts *ReadOptions, args []string) (string, error) {
	query, filePath := args[0], args[1]

	editor, err := hcledit.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %s", err)
	}

	readOpts := []hcledit.Option{}
	if opts.Fallback {
		readOpts = append(readOpts, hcledit.WithReadFallbackToRawString())
	}
	results, err := editor.Read(query, readOpts...)
	if err != nil && !opts.Fallback {
		return "", fmt.Errorf("failed to read file: %s", err)
	}

	converted := make(map[string]interface{})
	for k, v := range results {
		converted[k] = ctyToGo(v)
	}

	// Special case: for YAML output, if the value is a string that looks like an array (e.g. "[a b c]"), convert it to a slice
	if opts.OutputFormat == "yaml" {
		for k, v := range converted {
			if s, ok := v.(string); ok && strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
				// Remove brackets and split by space
				trimmed := strings.TrimSuffix(strings.TrimPrefix(s, "["), "]")
				if len(trimmed) > 0 {
					parts := strings.Fields(trimmed)
					converted[k] = parts
				} else {
					converted[k] = []string{}
				}
			}
		}
	}

	if strings.HasPrefix(opts.OutputFormat, "go-template") {
		return displayTemplate(opts.OutputFormat, converted)
	}

	switch opts.OutputFormat {
	case "json":
		j, err := json.Marshal(converted)
		return string(j), err
	case "yaml":
		y, err := yaml.Marshal(converted)
		return string(y), err
	default:
		return "", fmt.Errorf("invalid output-format: %s", opts.OutputFormat)
	}
}

func displayTemplate(format string, results map[string]interface{}) (string, error) {
	split := strings.SplitN(format, "=", 2)

	if len(split) != 2 {
		return "", errors.New("go-template should be passed as go-template='<TEMPLATE>'")
	}

	templateFormat := strings.Trim(split[1], "'")

	tmpl, err := template.New("output").Parse(templateFormat)
	if err != nil {
		return "", err
	}

	var result strings.Builder

	for key, value := range results {
		formatted := struct {
			Key   string
			Value string
		}{
			fmt.Sprintf("%v", key),
			fmt.Sprintf("%v", value),
		}

		if err := tmpl.Execute(&result, formatted); err != nil {
			return result.String(), err
		}
	}

	return result.String(), nil
}

// ctyToGo recursively converts cty.Value to Go native types.
func ctyToGo(val interface{}) interface{} {
	switch v := val.(type) {
	case nil:
		return nil
	case string, int, float64, bool:
		return v
	case fmt.Stringer:
		return v.String()
	}
	// Try cty.Value
	if _, ok := val.(interface{ Type() interface{} }); ok {
		typeName := fmt.Sprintf("%T", val)
		if strings.Contains(typeName, "cty.Value") {
			ctyVal := reflect.ValueOf(val)
			isNull := ctyVal.MethodByName("IsNull").Call(nil)[0].Bool()
			isKnown := ctyVal.MethodByName("IsKnown").Call(nil)[0].Bool()
			if isNull || !isKnown {
				return nil
			}
			canIter := ctyVal.MethodByName("CanIterateElements").Call(nil)[0].Bool()
			if canIter {
				asSlice := ctyVal.MethodByName("AsValueSlice").Call(nil)[0]
				res := make([]interface{}, asSlice.Len())
				for i := 0; i < asSlice.Len(); i++ {
					res[i] = ctyToGo(asSlice.Index(i).Interface())
				}
				return res
			}
			asMap := ctyVal.MethodByName("AsValueMap").Call(nil)[0]
			if asMap.Len() > 0 {
				res := make(map[string]interface{})
				for _, key := range asMap.MapKeys() {
					res[key.String()] = ctyToGo(asMap.MapIndex(key).Interface())
				}
				return res
			}
			// Only use AsString for primitive types
			asString := ctyVal.MethodByName("AsString").Call(nil)[0].String()
			// AsBigFloat
			asBigFloat := ctyVal.MethodByName("AsBigFloat").Call(nil)[0]
			if !asBigFloat.IsNil() {
				b := asBigFloat.Interface().(*big.Float)
				f, _ := b.Float64()
				return f
			}
			// Check if the value is a primitive type
			typeField := ctyVal.MethodByName("Type").Call(nil)[0]
			if typeField.String() == "cty.String" || typeField.String() == "cty.Number" || typeField.String() == "cty.Bool" {
				return asString
			}
			return nil
		}
	}
	return fmt.Sprintf("%v", val)
}
