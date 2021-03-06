package core

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/h2non/gock"
	"github.com/op/go-logging"
	v5 "github.com/retailcrm/api-client-go/v5"
	v1 "github.com/retailcrm/mg-transport-api-client-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

var (
	testCRMURL = "https://fake-uri.retailcrm.pro"
	testMGURL  = "https://mg-url.example.com"
)

type UtilsTest struct {
	suite.Suite
	utils *Utils
}

func mgClient() *v1.MgClient {
	return v1.New(testMGURL, "token")
}

func (u *UtilsTest) SetupSuite() {
	logger := NewLogger("code", logging.DEBUG, DefaultLogFormatter())
	awsConfig := ConfigAWS{
		AccessKeyID:     "access key id (will be removed)",
		SecretAccessKey: "secret access key",
		Region:          "region",
		Bucket:          "bucket",
		FolderName:      "folder",
		ContentType:     "image/jpeg",
	}

	u.utils = NewUtils(awsConfig, logger, false)
	u.utils.TokenCounter = 12345
}

func (u *UtilsTest) Test_ResetUtils() {
	assert.Equal(u.T(), "access key id (will be removed)", u.utils.ConfigAWS.AccessKeyID)
	assert.Equal(u.T(), uint32(12346), u.utils.TokenCounter)
	assert.False(u.T(), u.utils.IsDebug)

	awsConfig := u.utils.ConfigAWS
	awsConfig.AccessKeyID = "access key id"
	u.utils.resetUtils(awsConfig, true, 0)

	assert.Equal(u.T(), "access key id", u.utils.ConfigAWS.AccessKeyID)
	assert.Equal(u.T(), uint32(0), u.utils.TokenCounter)
	assert.True(u.T(), u.utils.IsDebug)
}

func (u *UtilsTest) Test_GenerateToken() {
	u.utils.TokenCounter = 12345
	token := u.utils.GenerateToken()
	assert.NotEmpty(u.T(), token)
	assert.Equal(u.T(), uint32(12346), u.utils.TokenCounter)
}

func (u *UtilsTest) Test_GetAPIClient_FailRuntime() {
	defer gock.Off()
	gock.New(testCRMURL)

	_, status, err := u.utils.GetAPIClient(testCRMURL, "key")
	assert.Equal(u.T(), http.StatusInternalServerError, status)
	assert.NotNil(u.T(), err)
}

func (u *UtilsTest) Test_GetAPIClient_FailAPI() {
	defer gock.Off()
	gock.New(testCRMURL).
		Get("/credentials").
		Reply(http.StatusBadRequest).
		BodyString(`{"success": false, "errorMsg": "error message"}`)

	_, status, err := u.utils.GetAPIClient(testCRMURL, "key")
	assert.Equal(u.T(), http.StatusBadRequest, status)
	if assert.NotNil(u.T(), err) {
		assert.Equal(u.T(), "invalid credentials", err.Error())
	}
}

func (u *UtilsTest) Test_GetAPIClient_FailAPICredentials() {
	resp := v5.CredentialResponse{
		Success:        true,
		Credentials:    []string{},
		SiteAccess:     "all",
		SitesAvailable: []string{},
	}

	data, _ := json.Marshal(resp)

	defer gock.Off()
	gock.New(testCRMURL).
		Get("/credentials").
		Reply(http.StatusOK).
		BodyString(string(data))

	_, status, err := u.utils.GetAPIClient(testCRMURL, "key")
	assert.Equal(u.T(), http.StatusBadRequest, status)
	if assert.NotNil(u.T(), err) {
		assert.Equal(u.T(), "missing credentials", err.Error())
	}
}

func (u *UtilsTest) Test_GetAPIClient_Success() {
	resp := v5.CredentialResponse{
		Success: true,
		Credentials: []string{
			"/api/integration-modules/{code}",
			"/api/integration-modules/{code}/edit",
		},
		SiteAccess:     "all",
		SitesAvailable: []string{"site"},
	}

	data, _ := json.Marshal(resp)

	defer gock.Off()
	gock.New(testCRMURL).
		Get("/credentials").
		Reply(http.StatusOK).
		BodyString(string(data))

	_, status, err := u.utils.GetAPIClient(testCRMURL, "key")
	require.NoError(u.T(), err)
	assert.Equal(u.T(), 0, status)
}

func (u *UtilsTest) Test_UploadUserAvatar_FailGet() {
	defer gock.Off()
	gock.New("https://example.com")

	uri, err := u.utils.UploadUserAvatar("https://example.com/image.jpg")
	assert.Empty(u.T(), uri)
	assert.Error(u.T(), err)
}

func (u *UtilsTest) Test_UploadUserAvatar_FailBadRequest() {
	defer gock.Off()
	gock.New("https://example.com").
		Get("/image.jpg").
		Reply(200).
		BodyString(`no image here`)

	uri, err := u.utils.UploadUserAvatar("https://example.com/image.jpg")
	assert.Empty(u.T(), uri)
	assert.Error(u.T(), err)
}

func (u *UtilsTest) Test_RemoveTrailingSlash() {
	assert.Equal(u.T(), testCRMURL, u.utils.RemoveTrailingSlash(testCRMURL+"/"))
	assert.Equal(u.T(), testCRMURL, u.utils.RemoveTrailingSlash(testCRMURL))
}

func TestUtils_GetMGItemData_FailRuntime_GetImage(t *testing.T) {
	defer gock.Off()
	gock.New(testMGURL)
	gock.New("https://example.com/")

	_, status, err := GetMGItemData(mgClient(), "https://example.com/item.jpg", "")
	assert.Error(t, err)
	assert.Equal(t, 0, status)
}

func TestUtils_GetMGItemData_FailAPI(t *testing.T) {
	defer gock.Off()

	gock.New("https://example.com/").
		Get("/item.jpg").
		Reply(200).
		BodyString(`fake data`)

	gock.New(testMGURL).
		Post("/files/upload_by_url").
		Reply(400).
		BodyString(`{"errors": ["error text"]}`)

	_, status, err := GetMGItemData(mgClient(), "https://example.com/item.jpg", "")
	assert.Error(t, err)
	assert.Equal(t, http.StatusBadRequest, status)
	assert.Equal(t, "error text", err.Error())
}

func TestUtils_GetMGItemData_Success(t *testing.T) {
	fileID := "file id"
	size := 40
	uri := "file uri"
	resp := v1.UploadFileResponse{
		ID:   fileID,
		Hash: "file hash",
		Type: "image/jpeg",
		Meta: v1.FileMeta{
			Width:  &size,
			Height: &size,
		},
		MimeType:  "image/jpeg",
		Size:      250,
		Url:       &uri,
		CreatedAt: time.Now(),
	}

	data, _ := json.Marshal(resp)

	defer gock.Off()

	gock.New("https://example.com/").
		Get("/item.jpg").
		Reply(200).
		BodyString(`fake data`)

	gock.New(testMGURL).
		Post("/files/upload_by_url").
		Reply(200).
		BodyString(string(data))

	response, status, err := GetMGItemData(mgClient(), "https://example.com/item.jpg", "caption")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, fileID, response.ID)
	assert.Equal(t, "caption", response.Caption)
}

func TestUtils_GetEntitySHA1(t *testing.T) {
	entity := struct {
		Field string
	}{
		Field: "value",
	}

	hash, err := GetEntitySHA1(entity)
	require.NoError(t, err)
	assert.Equal(t, "751b56fb98c9fd803140e8287b4236675554a668", hash)
}

func TestUtils_GetCurrencySymbol(t *testing.T) {
	for code, _ := range DefaultCurrencies() {
		if strings.ToUpper(code) == defaultCurrencies[code] {
			continue
		}

		assert.NotEqual(t, strings.ToUpper(code), GetCurrencySymbol(code))
	}

	assert.Equal(t, "XAG", GetCurrencySymbol("xag"))
}

func TestUtils_ReplaceMarkdownSymbols(t *testing.T) {
	test := "this *is* _test_ `string` [markdown"
	expected := "this \\*is\\* \\_test\\_ \\`string\\` \\[markdown"
	assert.Equal(t, expected, ReplaceMarkdownSymbols(test))
}

func TestUtils_Suite(t *testing.T) {
	suite.Run(t, new(UtilsTest))
}
