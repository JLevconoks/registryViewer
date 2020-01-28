package registry

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
)

type Registry struct {
	Protocol string
	BaseUrl  string
	SubPath  string
	client   *http.Client
}

type TokenTransport struct {
	Transport http.RoundTripper
}

type authToken struct {
	AuthToken string `json:"token"`
}

type repositoriesResponse struct {
	Repositories []string `json:"repositories"`
}

type tagsResponse struct {
	Tags []string `json:"tags"`
}

func (t *TokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.Transport.RoundTrip(req)
	return resp, err
}

func NewRegistry(protocol, baseUrl, subPath string) Registry {
	transport := &TokenTransport{http.DefaultTransport}
	c := &http.Client{
		Transport: transport,
	}
	return Registry{client: c,
		Protocol: protocol,
		BaseUrl:  baseUrl,
		SubPath:  subPath,
	}
}

func (r *Registry) ListRepositories() ([]string, error) {
	url := fmt.Sprintf("%v://%v/v2/_catalog?n=10000", r.Protocol, r.BaseUrl)
	resp, err := r.getWithAuth(url)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	var repos repositoriesResponse
	decoder := json.NewDecoder(resp.Body)
	err = decoder.Decode(&repos)
	if err != nil {
		return nil, err
	}

	return repos.Repositories, nil
}

func (r *Registry) Tags(name string) ([]string, error) {
	requestUrl := fmt.Sprintf("%v://%v/v2/%v%v/tags/list?n=10000", r.Protocol, r.BaseUrl, r.SubPath, name)

	resp, err := r.getWithAuth(requestUrl)
	if err != nil {
		return nil, err
	}

	var tags tagsResponse
	decoder := json.NewDecoder(resp.Body)
	err = decoder.Decode(&tags)
	if err != nil {
		return nil, err
	}

	return tags.Tags, nil
}

func (r *Registry) getWithAuth(url string) (*http.Response, error) {
	resp, err := r.client.Get(url)
	if err != nil {
		return resp, err
	}

	if resp.StatusCode == http.StatusUnauthorized {
		token := r.getAuthToken(resp)
		request, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		request.Header.Add("Authorization", "Bearer "+token.AuthToken)
		resp, err = r.client.Do(request)
	}

	return resp, err
}

func (r *Registry) getAuthToken(resp *http.Response) authToken {
	authHeaderValues := resp.Header[http.CanonicalHeaderKey("WWW-Authenticate")]
	if len(authHeaderValues) != 1 {
		log.Fatal("Auth header value is ", len(authHeaderValues))
	}
	index := strings.Index(authHeaderValues[0], " ")
	authHeaderValue := authHeaderValues[0][index+1:]
	params := strings.Split(authHeaderValue, ",")

	paramMap := make(map[string]string)

	for _, v := range params {
		index := strings.Index(v, "=")
		key := v[:index]
		value := v[index+2 : len(v)-1]
		paramMap[key] = value
	}

	request, err := http.NewRequest("GET", paramMap["realm"], nil)
	if err != nil {
		log.Fatal(err)
	}
	query := request.URL.Query()
	query.Add("service", paramMap["service"])
	query.Add("scope", paramMap["scope"])
	request.URL.RawQuery = query.Encode()
	response, err := r.client.Do(request)

	var token authToken
	decoder := json.NewDecoder(response.Body)
	err = decoder.Decode(&token)
	if err != nil {
		log.Fatal(err)
	}

	return token
}
