package plugin

import (
	"encoding/base64"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_GetQueryFromRequest(t *testing.T) {
	query := "SELECT * FROM users"
	// Get the size of the query and add 5 for the message type and size.
	size := int32(len(query) + 5)
	sizeBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(sizeBytes, uint32(size))
	data := append([]byte("Q"), sizeBytes...)
	data = append(data, []byte(query)...)
	// Encode the data to base64.
	request := base64.StdEncoding.EncodeToString(data)

	// Decode the request and extract the query.
	decodedQuery, err := GetQueryFromRequest(request)
	assert.Nil(t, err)
	assert.Equal(t, query, decodedQuery)
}

func Test_GetQueryFromRequest_Empty(t *testing.T) {
	// Decode the request and extract the query.
	decodedQuery, err := GetQueryFromRequest("")
	assert.Nil(t, err)
	assert.Equal(t, "", decodedQuery)
}

func Test_GetQueryFromRequest_Invalid(t *testing.T) {
	// Decode the request and extract the query.
	decodedQuery, err := GetQueryFromRequest("invalid")
	assert.NotNil(t, err)
	assert.Equal(t, "", decodedQuery)
}

func Test_GetQueryFromRequest_Short(t *testing.T) {
	// Decode the request and extract the query.
	decodedQuery, err := GetQueryFromRequest("Q")
	assert.NotNil(t, err)
	assert.Equal(t, "", decodedQuery)
}

func Test_GetQueryFromRequest_Shorter(t *testing.T) {
	// Decode the request and extract the query.
	decodedQuery, err := GetQueryFromRequest("QAAAA")
	assert.NotNil(t, err)
	assert.Equal(t, "", decodedQuery)
}
