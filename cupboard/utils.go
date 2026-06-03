package cupboard

import (
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

// Logging and error handling utilities
func NoError(err error, args ...any) {
	if err != nil {
		log.Fatal().Str("module", "DB").Err(err).Msg(fmt.Sprint(args...))
	}
}

func LogError(err error, msg string, args ...any) {
	if err != nil {
		log.Error().Str("module", "DB").Err(err).Msgf(msg, args...)
	}
}

func LogWarning(err error, msg string, args ...any) {
	if err != nil {
		log.Warn().Str("module", "DB").Err(err).Msgf(msg, args...)
	}
}

func LogQueryResult(items any, err error) {
	if err != nil {
		log.Error().Str("module", "DB").Err(err).Msg("query failed")
	} else {
		log.Debug().Str("module", "DB").Interface("result", items).Msg("query succeeded")
	}
}

func LogQuery(query string, args pgx.NamedArgs) {
	evt := log.Debug().Str("module", "DB").Str("sql", query)
	for key, value := range args {
		if key != "embedding" {
			evt = evt.Interface(key, value)
		} else {
			evt = evt.Str("embedding", "[REDACTED]") // Avoid logging large embeddings
		}
	}
	evt.Msg("query")
}

func ConcatArray[T any](arrays ...[]T) []T {
	var result []T
	for _, array := range arrays {
		if array != nil {
			result = append(result, array...)
		}
	}
	return result
}

// SQL to Go type conversions for nullable fields and custom types
// func nullStringToString(ns sql.NullString) string {
// 	if ns.Valid {
// 		return ns.String
// 	}
// 	return ""
// }

// func nullTimeToTime(nt sql.NullTime) time.Time {
// 	if nt.Valid {
// 		return nt.Time
// 	}
// 	return time.Time{}
// }

// func nullBigIntToInt64(ni sql.NullInt64) int64 {
// 	if ni.Valid {
// 		return ni.Int64
// 	}
// 	return 0
// }

// // SQL marshalling and unmarshalling for vector and string array types
// type sqlVector []float32
// type sqlStringArray []string

// func (vec sqlStringArray) Value() (driver.Value, error) {
// 	bytes, err := json.Marshal(vec)
// 	return driver.Value(string(bytes)), err
// }

// func (vec *sqlStringArray) Scan(value interface{}) error {
// 	if value == nil {
// 		*vec = nil
// 		return nil
// 	}

// 	switch value := value.(type) {
// 	case []interface{}:
// 		converted := make([]string, len(value))
// 		for i, val := range value {
// 			converted[i] = val.(string)
// 		}
// 		*vec = converted
// 	case []byte:
// 	case string:
// 		return json.Unmarshal([]byte(value), vec)
// 	default:
// 		return fmt.Errorf("unsupported type: %T", value)
// 	}
// 	return nil
// }

// func (vec sqlVector) Value() (driver.Value, error) {
// 	if len(vec) == 0 {
// 		return driver.Value(nil), fmt.Errorf("vector cannot be nil or empty")
// 	}
// 	bytes, err := json.Marshal(vec)
// 	return driver.Value(string(bytes)), err
// }

// func (vec *sqlVector) Scan(value interface{}) error {
// 	if value == nil {
// 		*vec = nil
// 		return nil
// 	}

// 	switch value := value.(type) {
// 	case []interface{}:
// 		converted := make([]float32, len(value))
// 		for i, val := range value {
// 			switch v := val.(type) {
// 			case float64:
// 				converted[i] = float32(v)
// 			case float32:
// 				converted[i] = v
// 			case int:
// 				converted[i] = float32(v)
// 			default:
// 				return fmt.Errorf("unsupported array element type: %T", val)
// 			}
// 		}
// 		*vec = converted
// 		return nil
// 	case []float32:
// 		*vec = value
// 		return nil
// 	case []float64:
// 		converted := make([]float32, len(value))
// 		for i, v := range value {
// 			converted[i] = float32(v)
// 		}
// 		*vec = converted
// 		return nil
// 	case []int:
// 		converted := make([]float32, len(value))
// 		for i, val := range value {
// 			converted[i] = float32(val)
// 		}
// 		*vec = converted
// 		return nil
// 	case []byte:
// 		return json.Unmarshal(value, vec)
// 	case string:
// 		return json.Unmarshal([]byte(value), vec)
// 	default:
// 		return fmt.Errorf("unsupported type: %T", value)
// 	}
// }
