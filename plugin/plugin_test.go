package plugin

import (
	"context"
	"encoding/base64"
	"os"
	"testing"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/gatewayd-io/gatewayd-plugin-sdk/logging"
	"github.com/go-redis/redis/v8"
	"github.com/hashicorp/go-hclog"
	pgproto3 "github.com/jackc/pgx/v5/pgproto3"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/structpb"
)

func testQueryRequest() (string, string) {
	query := "SELECT * FROM users"
	queryMsg := pgproto3.Query{String: query}
	// Encode the data to base64.
	return query, base64.StdEncoding.EncodeToString(queryMsg.Encode(nil))
}

func testStartupRequest() string {
	startupMsg := pgproto3.StartupMessage{
		ProtocolVersion: 196608,
		Parameters: map[string]string{
			"user":     "postgres",
			"database": "postgres",
		},
	}
	return base64.StdEncoding.EncodeToString(startupMsg.Encode(nil))
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

	// Create and initialize a new plugin.
	logger := hclog.New(&hclog.LoggerOptions{
		Level:  logging.GetLogLevel("error"),
		Output: os.Stdout,
	})
	p := NewCachePlugin(Plugin{
		Logger:      logger,
		RedisURL:    redisURL,
		RedisClient: redisClient,
	})
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
	startupReq := testStartupRequest()
	args := map[string]interface{}{
		"request": startupReq,
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
	req, err := structpb.NewStruct(args)
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
	req, err = structpb.NewStruct(args)
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
	response := "VAAAABsAAWlkAAAAQAQAAQAAABcABP////8AAEQAAAALAAEAAAABMUMAAAANU0VMRUNUIDEAWgAAAAVJ"
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
	resp, err := structpb.NewStruct(args)
	assert.Nil(t, err)
	result, err = p.Impl.OnTrafficFromServer(context.Background(), resp)
	assert.Nil(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, result, resp)

	// Check that the query and response was cached.
	cachedResponse := redisClient.Get(
		context.Background(), "localhost:5432:postgres:"+request).Val()
	assert.Equal(t, cachedResponse, response)

	// Test the plugin's OnTrafficFromClient method with a cached response.
	result, err = p.Impl.OnTrafficFromClient(context.Background(), req)
	assert.Nil(t, err)
	assert.NotNil(t, result)
	resultMap := result.AsMap()
	assert.Equal(t, resultMap["response"], response)
	assert.Equal(t, resultMap["terminate"], true)
}
