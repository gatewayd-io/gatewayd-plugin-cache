package plugin

import (
	"context"
	"encoding/base64"
	"os"
	"sync"
	"testing"

	miniredis "github.com/alicebob/miniredis/v2"
	sdkAct "github.com/gatewayd-io/gatewayd-plugin-sdk/act"
	"github.com/gatewayd-io/gatewayd-plugin-sdk/logging"
	v1 "github.com/gatewayd-io/gatewayd-plugin-sdk/plugin/v1"
	"github.com/go-redis/redis/v8"
	"github.com/hashicorp/go-hclog"
	pgproto3 "github.com/jackc/pgx/v5/pgproto3"
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
