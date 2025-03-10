package rehydrate

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"time"
)

const (
	UNDEFINED         = -1
	HOLE              = -2
	NAN               = -3
	POSITIVE_INFINITY = -4
	NEGATIVE_INFINITY = -5
	NEGATIVE_ZERO     = -6
)

type ReviverFunc func(interface{}) (interface{}, error)

func Parse(serialized string, revivers map[string]ReviverFunc) (interface{}, error) {
	var parsed interface{}
	if err := json.Unmarshal([]byte(serialized), &parsed); err != nil {
		return nil, err
	}

	var hydrate func(index int, standalone bool, values []interface{}, computed []bool, revivers map[string]ReviverFunc) (interface{}, error)

	if num, ok := parsed.(float64); ok {
		return hydrate(int(num), true, nil, nil, revivers)
	}

	values, ok := parsed.([]interface{})
	if !ok || len(values) == 0 {
		return nil, errors.New("invalid input")
	}

	hydrated := make([]interface{}, len(values))
	computed := make([]bool, len(values))

	hydrate = func(index int, standalone bool, values []interface{}, computed []bool, revivers map[string]ReviverFunc) (interface{}, error) {
		switch index {
		case UNDEFINED:
			return nil, nil
		case NAN:
			return math.NaN(), nil
		case POSITIVE_INFINITY:
			return math.Inf(1), nil
		case NEGATIVE_INFINITY:
			return math.Inf(-1), nil
		case NEGATIVE_ZERO:
			return math.Copysign(0, -1), nil
		}

		if standalone {
			return nil, errors.New("invalid input")
		}

		if computed[index] {
			return hydrated[index], nil
		}

		value := values[index]

		switch v := value.(type) {
		case nil, bool, float64, string:
			hydrated[index] = v
			computed[index] = true
			return v, nil
		}

		if arr, ok := value.([]interface{}); ok {
			if len(arr) > 0 {
				if typeStr, ok := arr[0].(string); ok {
					if revivers != nil {
						if reviver, exists := revivers[typeStr]; exists {
							innerVal, err := hydrate(getInt(arr, 1), false, values, computed, revivers)
							if err != nil {
								return nil, err
							}
							res, err := reviver(innerVal)
							if err != nil {
								return nil, err
							}
							hydrated[index] = res
							computed[index] = true
							return res, nil
						}
					}

					switch typeStr {
					case "Date":
						dateStr, ok := arr[1].(string)
						if !ok {
							return nil, errors.New("invalid Date format")
						}
						t, err := time.Parse(time.RFC3339, dateStr)
						if err != nil {
							return nil, err
						}
						hydrated[index] = t
						computed[index] = true
						return t, nil

					case "Set":
						set := make(map[interface{}]struct{})
						hydrated[index] = set
						computed[index] = true
						for i := 1; i < len(arr); i++ {
							elemIndex, err := toInt(arr[i])
							if err != nil {
								return nil, err
							}
							elem, err := hydrate(elemIndex, false, values, computed, revivers)
							if err != nil {
								return nil, err
							}
							set[elem] = struct{}{}
						}
						return set, nil

					case "Map":
						m := make(map[interface{}]interface{})
						hydrated[index] = m
						computed[index] = true
						for i := 1; i < len(arr); i += 2 {
							keyIndex, err := toInt(arr[i])
							if err != nil {
								return nil, err
							}
							valIndex, err := toInt(arr[i+1])
							if err != nil {
								return nil, err
							}
							key, err := hydrate(keyIndex, false, values, computed, revivers)
							if err != nil {
								return nil, err
							}
							val, err := hydrate(valIndex, false, values, computed, revivers)
							if err != nil {
								return nil, err
							}
							m[key] = val
						}
						return m, nil

					case "RegExp":
						pattern, ok1 := arr[1].(string)
						_, ok2 := arr[2].(string)
						if !ok1 || !ok2 {
							return nil, errors.New("invalid RegExp format")
						}
						re, err := regexp.Compile(pattern)
						if err != nil {
							return nil, err
						}
						hydrated[index] = re
						computed[index] = true
						return re, nil

					case "Object":
						hydrated[index] = arr[1]
						computed[index] = true
						return arr[1], nil

					case "BigInt":
						bigStr, ok := arr[1].(string)
						if !ok {
							return nil, errors.New("invalid BigInt format")
						}
						bigInt := new(big.Int)
						_, ok = bigInt.SetString(bigStr, 10)
						if !ok {
							return nil, errors.New("failed to parse BigInt")
						}
						hydrated[index] = bigInt
						computed[index] = true
						return bigInt, nil

					case "null":
						obj := make(map[string]interface{})
						hydrated[index] = obj
						computed[index] = true
						for i := 1; i < len(arr); i += 2 {
							key, ok := arr[i].(string)
							if !ok {
								return nil, errors.New("invalid key in null object")
							}
							valIndex, err := toInt(arr[i+1])
							if err != nil {
								return nil, err
							}
							val, err := hydrate(valIndex, false, values, computed, revivers)
							if err != nil {
								return nil, err
							}
							obj[key] = val
						}
						return obj, nil

					case "Int8Array", "Uint8Array", "Uint8ClampedArray",
						"Int16Array", "Uint16Array", "Int32Array", "Uint32Array",
						"Float32Array", "Float64Array", "BigInt64Array", "BigUint64Array":
						b64, ok := arr[1].(string)
						if !ok {
							return nil, errors.New("invalid typed array format")
						}
						data, err := base64.StdEncoding.DecodeString(b64)
						if err != nil {
							return nil, err
						}
						hydrated[index] = data
						computed[index] = true
						return data, nil

					case "ArrayBuffer":
						b64, ok := arr[1].(string)
						if !ok {
							return nil, errors.New("invalid ArrayBuffer format")
						}
						data, err := base64.StdEncoding.DecodeString(b64)
						if err != nil {
							return nil, err
						}
						hydrated[index] = data
						computed[index] = true
						return data, nil

					default:
						return nil, fmt.Errorf("unknown type %s", typeStr)
					}
				}
			}
			arrResult := make([]interface{}, len(arr))
			hydrated[index] = arrResult
			computed[index] = true
			for i, item := range arr {
				if num, err := toInt(item); err == nil && num == HOLE {
					continue
				}
				itemIndex, err := toInt(item)
				if err != nil {
					return nil, err
				}
				elem, err := hydrate(itemIndex, false, values, computed, revivers)
				if err != nil {
					return nil, err
				}
				arrResult[i] = elem
			}
			return arrResult, nil
		}

		if obj, ok := value.(map[string]interface{}); ok {
			result := make(map[string]interface{})
			hydrated[index] = result
			computed[index] = true
			for key, val := range obj {
				valIndex, err := toInt(val)
				if err != nil {
					return nil, err
				}
				hVal, err := hydrate(valIndex, false, values, computed, revivers)
				if err != nil {
					return nil, err
				}
				result[key] = hVal
			}
			return result, nil
		}

		return nil, errors.New("unknown value type")
	}

	return hydrate(0, false, values, computed, revivers)
}

func toInt(v interface{}) (int, error) {
	switch num := v.(type) {
	case float64:
		return int(num), nil
	case int:
		return num, nil
	case string:
		i, err := strconv.Atoi(num)
		if err != nil {
			return 0, err
		}
		return i, nil
	default:
		return 0, errors.New("unable to convert to int")
	}
}

func getInt(arr []interface{}, i int) int {
	val, _ := toInt(arr[i])
	return val
}

type Revivers map[string]ReviverFunc

func ConvertUnsupportedTypes(v interface{}) interface{} {
	switch value := v.(type) {
	case map[interface{}]struct{}:
		arr := make([]interface{}, 0, len(value))
		for key := range value {
			arr = append(arr, ConvertUnsupportedTypes(key))
		}
		return arr
	case []interface{}:
		for i, item := range value {
			value[i] = ConvertUnsupportedTypes(item)
		}
		return value
	case map[string]interface{}:
		for k, item := range value {
			value[k] = ConvertUnsupportedTypes(item)
		}
		return value
	case map[interface{}]interface{}:
		m := make(map[string]interface{})
		for key, item := range value {
			m[fmt.Sprintf("%v", key)] = ConvertUnsupportedTypes(item)
		}
		return m
	default:
		return v
	}
}

func Rehydrate(inputString string) (string, error) {
	result, err := Parse(inputString, Revivers{
		"Reactive": func(val interface{}) (interface{}, error) {
			return val, nil
		},
		"Ref": func(val interface{}) (interface{}, error) {
			return val, nil
		},
		"EmptyRef": func(val interface{}) (interface{}, error) {
			return val, nil
		},
	})
	if err != nil {
		return "", err
	}

	fixedResult := ConvertUnsupportedTypes(result)

	jsonOutput, err := json.MarshalIndent(fixedResult, "", "  ")
	if err != nil {
		return "", err
	}

	return string(jsonOutput), nil
}
