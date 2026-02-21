package plugin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	sdkAct "github.com/gatewayd-io/gatewayd-plugin-sdk/act"
	"github.com/gatewayd-io/gatewayd-plugin-sdk/logging"
	v1 "github.com/gatewayd-io/gatewayd-plugin-sdk/plugin/v1"
	"github.com/hashicorp/go-hclog"
	pgproto3 "github.com/jackc/pgx/v5/pgproto3"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

func testQueryRequest() (string, []byte) {
	query := "SELECT * FROM users"
	queryMsg := pgproto3.Query{String: query}
	// Encode the data to base64.
	queryBytes, _ := queryMsg.Encode(nil)
	return query, queryBytes
}

func testQueryRequestWithDateFunction() (string, []byte) {
	query := `SELECT
    	user_id,
    	username,
    	last_login,
    	NOW() AS current_time
		FROM
    	users
		WHERE
    	last_login >= CURRENT_DATE;`
	queryMsg := pgproto3.Query{String: query}
	// Encode the data to base64.
	queryBytes, _ := queryMsg.Encode(nil)
	return query, queryBytes
}

func testStartupRequest() []byte {
	startupMsg := pgproto3.StartupMessage{
		ProtocolVersion: 196608,
		Parameters: map[string]string{
			"user":     "postgres",
			"database": "postgres",
		},
	}
	startupMsgBytes, _ := startupMsg.Encode(nil)
	return startupMsgBytes
}

func Test_Plugin(t *testing.T) {
	// Initialize a new mock Redis server.
	redisServer := miniredis.RunT(t)
	assert.NotNil(t, redisServer)
	redisURL := "redis://" + redisServer.Addr() + "/0"
	redisConfig, err := redis.ParseURL(redisURL)
	assert.Nil(t, err)
	assert.NotNil(t, redisConfig)
	redisClient := redis.NewClient(redisConfig)
	assert.NotNil(t, redisClient)

	updateCacheChannel := make(chan *v1.Struct, 10)

	// Create and initialize a new plugin.
	logger := hclog.New(&hclog.LoggerOptions{
		Level:  logging.GetLogLevel("error"),
		Output: os.Stdout,
	})
	p := NewCachePlugin(Plugin{
		Logger:             logger,
		RedisURL:           redisURL,
		RedisClient:        redisClient,
		UpdateCacheChannel: updateCacheChannel,
		WaitGroup:          &sync.WaitGroup{},
	})

	p.Impl.WaitGroup.Add(1)
	go p.Impl.UpdateCache(context.Background())

	assert.NotNil(t, p)

	// Test the plugin's GetPluginConfig method.
	config, err := p.Impl.GetPluginConfig(context.Background(), nil)
	assert.Nil(t, err)
	assert.NotNil(t, config)
	configMap := config.AsMap()
	assert.Equal(t, configMap["id"], PluginConfig["id"])
	assert.Equal(t, configMap["description"], PluginConfig["description"])
	assert.Equal(t, configMap["authors"], PluginConfig["authors"])
	assert.Equal(t, configMap["license"], PluginConfig["license"])
	assert.Equal(t, configMap["projectUrl"], PluginConfig["projectUrl"])
	assert.Equal(t, configMap["config"], PluginConfig["config"])
	assert.InDeltaSlice(t, configMap["hooks"], PluginConfig["hooks"], 0)
	assert.Equal(t, configMap["tags"], PluginConfig["tags"])
	assert.Equal(t, configMap["categories"], PluginConfig["categories"])

	// Test the plugin's OnTrafficFromClient method with a StartupMessage.
	args := map[string]interface{}{
		"request": testStartupRequest(),
		"client": map[string]interface{}{
			"local":  "localhost:15432",
			"remote": "localhost:45320",
		},
		"server": map[string]interface{}{
			"local":  "localhost:54321",
			"remote": "localhost:5432",
		},
		"error": "",
	}
	req, err := v1.NewStruct(args)
	assert.Nil(t, err)
	result, err := p.Impl.OnTrafficFromClient(context.Background(), req)
	assert.Nil(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, result, req)

	// Check that the database name was cached.
	database := redisClient.Get(context.Background(), "localhost:45320").Val()
	assert.Equal(t, database, "postgres")

	// Test the plugin's OnTrafficFromClient method.
	_, request := testQueryRequest()
	args = map[string]interface{}{
		"request": request,
		"client": map[string]interface{}{
			"local":  "localhost:15432",
			"remote": "localhost:45320",
		},
		"server": map[string]interface{}{
			"local":  "localhost:54321",
			"remote": "localhost:5432",
		},
		"error": "",
	}
	req, err = v1.NewStruct(args)
	assert.Nil(t, err)
	result, err = p.Impl.OnTrafficFromClient(context.Background(), req)
	assert.Nil(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, result, req)

	// Test the plugin's OnTrafficFromServer method.
	/*
		postgres=# select * from test limit 1;
		id
		----
		1
		(1 row)
	*/
	response, err := base64.StdEncoding.DecodeString("VAAAABsAAWlkAAAAQAQAAQAAABcABP////8AAEQAAAALAAEAAAABMUMAAAANU0VMRUNUIDEAWgAAAAVJ")
	assert.Nil(t, err)
	args = map[string]interface{}{
		"request":  request,
		"response": response,
		"client": map[string]interface{}{
			"local":  "localhost:15432",
			"remote": "localhost:45320",
		},
		"server": map[string]interface{}{
			"local":  "localhost:54321",
			"remote": "localhost:5432",
		},
		"error": "",
	}
	resp, err := v1.NewStruct(args)
	assert.Nil(t, err)
	result, err = p.Impl.OnTrafficFromServer(context.Background(), resp)
	assert.Nil(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, result, resp)

	close(updateCacheChannel)
	p.Impl.WaitGroup.Wait()

	// Check that the query and response was cached.
	cachedResponse, err := redisClient.Get(
		context.Background(), "localhost:5432:postgres:"+string(request)).Bytes()
	assert.Nil(t, err)
	assert.Equal(t, cachedResponse, response)

	// Test the plugin's OnTrafficFromClient method with a cached response.
	result, err = p.Impl.OnTrafficFromClient(context.Background(), req)
	assert.Nil(t, err)
	assert.NotNil(t, result)
	resultMap := result.AsMap()
	assert.Equal(t, resultMap["response"], response)
	assert.Contains(t, resultMap, sdkAct.Signals)
}

func TestPluginDateFunctionInQuery(t *testing.T) {
	// Initialize a new mock Redis server.
	mockRedisServer := miniredis.RunT(t)
	redisURL := "redis://" + mockRedisServer.Addr() + "/0"
	redisConfig, err := redis.ParseURL(redisURL)
	redisClient := redis.NewClient(redisConfig)

	cacheUpdateChannel := make(chan *v1.Struct, 10)

	// Create and initialize a new plugin.
	logger := hclog.New(&hclog.LoggerOptions{
		Level:  logging.GetLogLevel("error"),
		Output: os.Stdout,
	})
	plugin := NewCachePlugin(Plugin{
		Logger:             logger,
		RedisURL:           redisURL,
		RedisClient:        redisClient,
		UpdateCacheChannel: cacheUpdateChannel,
		WaitGroup:          &sync.WaitGroup{},
	})

	plugin.Impl.WaitGroup.Add(1)
	go plugin.Impl.UpdateCache(context.Background())

	// Test the plugin's OnTrafficFromClient method with a StartupMessage.
	clientArgs := map[string]interface{}{
		"request": testStartupRequest(),
		"client": map[string]interface{}{
			"local":  "localhost:15432",
			"remote": "localhost:45320",
		},
		"server": map[string]interface{}{
			"local":  "localhost:54321",
			"remote": "localhost:5432",
		},
		"error": "",
	}
	clientRequest, err := v1.NewStruct(clientArgs)
	plugin.Impl.OnTrafficFromClient(context.Background(), clientRequest)

	// Test the plugin's OnTrafficFromServer method with a query request.
	_, queryRequest := testQueryRequestWithDateFunction()
	queryResponse, err := base64.StdEncoding.DecodeString("VAAAABsAAWlkAAAAQAQAAQAAABcABP////8AAEQAAAALAAEAAAABMUMAAAANU0VMRUNUIDEAWgAAAAVJ")
	assert.Nil(t, err)
	queryArgs := map[string]interface{}{
		"request":  queryRequest,
		"response": queryResponse,
		"client": map[string]interface{}{
			"local":  "localhost:15432",
			"remote": "localhost:45320",
		},
		"server": map[string]interface{}{
			"local":  "localhost:54321",
			"remote": "localhost:5432",
		},
		"error": "",
	}
	serverRequest, err := v1.NewStruct(queryArgs)
	plugin.Impl.OnTrafficFromServer(context.Background(), serverRequest)

	close(cacheUpdateChannel)
	plugin.Impl.WaitGroup.Wait()

	keys, _ := redisClient.Keys(context.Background(), "*").Result()
	assert.Equal(t, 1, len(keys)) // Only one key (representing the database name) should be present.
}

func newTestPlugin(t *testing.T) (*CachePlugin, *redis.Client) {
	t.Helper()
	redisServer := miniredis.RunT(t)
	redisURL := "redis://" + redisServer.Addr() + "/0"
	redisConfig, _ := redis.ParseURL(redisURL)
	redisClient := redis.NewClient(redisConfig)

	logger := hclog.New(&hclog.LoggerOptions{
		Level:  logging.GetLogLevel("error"),
		Output: os.Stdout,
	})
	p := NewCachePlugin(Plugin{
		Logger:             logger,
		RedisURL:           redisURL,
		RedisClient:        redisClient,
		Expiry:             time.Hour,
		ScanCount:          1000,
		UpdateCacheChannel: make(chan *v1.Struct, 10),
		WaitGroup:          &sync.WaitGroup{},
	})
	return p, redisClient
}

func TestIsCacheNeeded(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  bool
	}{
		{"plain SELECT", "SELECT * FROM USERS", true},
		{"contains NOW()", "SELECT NOW(), ID FROM USERS", false},
		{"contains CURRENT_DATE", "SELECT ID FROM USERS WHERE CREATED_AT >= CURRENT_DATE", false},
		{"contains CURRENT_TIMESTAMP", "SELECT CURRENT_TIMESTAMP", false},
		{"contains CLOCK_TIMESTAMP()", "SELECT CLOCK_TIMESTAMP()", false},
		{"contains LOCALTIME", "SELECT LOCALTIME", false},
		{"contains LOCALTIMESTAMP", "SELECT LOCALTIMESTAMP", false},
		{"contains AGE()", "SELECT AGE(TIMESTAMP '2001-04-10')", false},
		{"contains STATEMENT_TIMESTAMP", "SELECT STATEMENT_TIMESTAMP()", false},
		{"contains TIMEOFDAY", "SELECT TIMEOFDAY()", false},
		{"contains TRANSACTION_TIMESTAMP", "SELECT TRANSACTION_TIMESTAMP()", false},
		{"INSERT query", "INSERT INTO USERS VALUES (1)", true},
		{"empty query", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsCacheNeeded(tt.query))
		})
	}
}

func TestOnClosed(t *testing.T) {
	p, redisClient := newTestPlugin(t)
	ctx := context.Background()

	// Simulate a stored client-to-database mapping.
	redisClient.Set(ctx, "localhost:45320", "postgres", 0)
	val := redisClient.Get(ctx, "localhost:45320").Val()
	assert.Equal(t, "postgres", val)

	// Call OnClosed to clean up.
	args := map[string]interface{}{
		"client": map[string]interface{}{
			"local":  "localhost:15432",
			"remote": "localhost:45320",
		},
	}
	req, err := v1.NewStruct(args)
	assert.Nil(t, err)

	result, err := p.Impl.OnClosed(ctx, req)
	assert.Nil(t, err)
	assert.NotNil(t, result)

	// The client key should be deleted.
	val = redisClient.Get(ctx, "localhost:45320").Val()
	assert.Equal(t, "", val)
}

func TestOnClosedNilClient(t *testing.T) {
	p, _ := newTestPlugin(t)
	ctx := context.Background()

	args := map[string]interface{}{}
	req, err := v1.NewStruct(args)
	assert.Nil(t, err)

	result, err := p.Impl.OnClosed(ctx, req)
	assert.Nil(t, err)
	assert.NotNil(t, result)
}

func TestInvalidateDML(t *testing.T) {
	p, redisClient := newTestPlugin(t)
	ctx := context.Background()

	_, request := testQueryRequest()
	cacheKey := "localhost:5432:postgres:" + string(request)
	tableKey := "users:" + cacheKey

	// Pre-populate cache entries (simulating cached SELECT response + table index).
	redisClient.Set(ctx, cacheKey, "cached-response-data", time.Hour)
	redisClient.Set(ctx, tableKey, "", time.Hour)

	val := redisClient.Get(ctx, cacheKey).Val()
	assert.Equal(t, "cached-response-data", val)

	// Build a base64-encoded query message for an INSERT query.
	insertQuery := map[string]string{"String": "INSERT INTO users VALUES (1)"}
	queryJSON, err := json.Marshal(insertQuery)
	assert.Nil(t, err)
	encodedQuery := base64.StdEncoding.EncodeToString(queryJSON)

	p.Impl.invalidateDML(ctx, encodedQuery)

	// Both the cached response and the table index key should be deleted.
	val = redisClient.Get(ctx, cacheKey).Val()
	assert.Equal(t, "", val)
	val = redisClient.Get(ctx, tableKey).Val()
	assert.Equal(t, "", val)
}

func TestInvalidateDMLSelectIgnored(t *testing.T) {
	p, redisClient := newTestPlugin(t)
	ctx := context.Background()

	// Pre-populate a cache entry.
	redisClient.Set(ctx, "test-key", "test-value", time.Hour)

	selectQuery := map[string]string{"String": "SELECT * FROM users"}
	queryJSON, _ := json.Marshal(selectQuery)
	encodedQuery := base64.StdEncoding.EncodeToString(queryJSON)

	p.Impl.invalidateDML(ctx, encodedQuery)

	// SELECT queries should not trigger invalidation.
	val := redisClient.Get(ctx, "test-key").Val()
	assert.Equal(t, "test-value", val)
}

func TestUpdateCacheContinuesOnError(t *testing.T) {
	p, redisClient := newTestPlugin(t)
	ctx := context.Background()

	p.Impl.WaitGroup.Add(1)
	go p.Impl.UpdateCache(ctx)

	// Send a message with empty database (should trigger continue, not kill goroutine).
	badArgs := map[string]interface{}{
		"request":  []byte{},
		"response": []byte{},
		"client": map[string]interface{}{
			"remote": "unknown:99999",
		},
		"server": map[string]interface{}{
			"remote": "localhost:5432",
		},
	}
	badResp, _ := v1.NewStruct(badArgs)
	p.Impl.UpdateCacheChannel <- badResp

	// Now send a valid message. If the goroutine survived, this will be processed.
	_, request := testQueryRequest()
	response, _ := base64.StdEncoding.DecodeString(
		"VAAAABsAAWlkAAAAQAQAAQAAABcABP////8AAEQAAAALAAEAAAABMUMAAAANU0VMRUNUIDEAWgAAAAVJ")

	// Set up the database mapping so UpdateCache can find it.
	redisClient.Set(ctx, "localhost:45320", "postgres", 0)

	goodArgs := map[string]interface{}{
		"request":  request,
		"response": response,
		"client": map[string]interface{}{
			"remote": "localhost:45320",
		},
		"server": map[string]interface{}{
			"remote": "localhost:5432",
		},
	}
	goodResp, _ := v1.NewStruct(goodArgs)
	p.Impl.UpdateCacheChannel <- goodResp

	close(p.Impl.UpdateCacheChannel)
	p.Impl.WaitGroup.Wait()

	// The valid message should have been cached (goroutine survived the error).
	cachedResponse, err := redisClient.Get(
		ctx, "localhost:5432:postgres:"+string(request)).Bytes()
	assert.Nil(t, err)
	assert.Equal(t, response, cachedResponse)
}
