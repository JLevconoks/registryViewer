package registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
)

type Registry struct {
	Protocol        string
	BaseUrl         string
	SubPath         string
	client          *http.Client
	paginationLimit int
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

func NewRegistry(protocol, baseUrl, subPath string, paginationLimit int) Registry {
	transport := &TokenTransport{http.DefaultTransport}
	c := &http.Client{
		Transport: transport,
	}
	return Registry{
		client:          c,
		Protocol:        protocol,
		BaseUrl:         baseUrl,
		SubPath:         subPath,
		paginationLimit: paginationLimit,
	}
}

func (r *Registry) ListRepositories() ([]string, error) {
	repositories := make([]string, 0, r.paginationLimit)
	baseUrl := fmt.Sprintf("%v://%v", r.Protocol, r.BaseUrl)
	next := fmt.Sprintf("/v2/_catalog?n=%d", r.paginationLimit)

	for next != "" {
		responseRepositories, link, err := r.getNextRepositories(baseUrl, next)
		if err != nil {
			return nil, err
		}

		next, err = parseLink(link)
		if err != nil {
			return nil, err
		}

		repositories = append(repositories, responseRepositories...)
	}

	sort.Strings(repositories)
	return repositories, nil
}

func (r *Registry) Tags(name string) ([]string, error) {
	tags := make([]string, 0, r.paginationLimit)
	baseUrl := fmt.Sprintf("%v://%v", r.Protocol, r.BaseUrl)
	next := fmt.Sprintf("/v2/%v%v/tags/list?n=%d", r.SubPath, name, r.paginationLimit)

	for next != "" {
		responseTags, link, err := r.getNextTags(baseUrl, next)
		if err != nil {
			return nil, err
		}

		next, err = parseLink(link)
		if err != nil {
			return nil, err
		}

		tags = append(tags, responseTags...)
	}

	return tags, nil
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
		return r.client.Do(request)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		buf := new(strings.Builder)
		n, _ := io.Copy(buf, resp.Body)
		if n > 0 {
			return nil, errors.New(buf.String())
		}
		return nil, errors.New("unable to fetch")
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
	if err != nil {
		log.Fatal(err)
	}

	var token authToken
	decoder := json.NewDecoder(response.Body)
	err = decoder.Decode(&token)
	if err != nil {
		log.Fatal(err)
	}

	return token
}

func parseLink(link string) (string, error) {
	if link == "" {
		return link, nil
	}

	start := strings.Index(link, "/v2/")
	if start == -1 {
		return "", fmt.Errorf("link header must contain '/v2/' [%s]", link)
	}

	end := strings.Index(link, ">")
	if end == -1 {
		return "", fmt.Errorf("link header must contain '>' [%s]", link)
	}

	return link[start:end], nil
}

func (r *Registry) getNextRepositories(baseUrl string, next string) ([]string, string, error) {
	url := fmt.Sprintf("%v%v", baseUrl, next)
	resp, err := r.getWithAuth(url)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	var repos repositoriesResponse
	decoder := json.NewDecoder(resp.Body)
	err = decoder.Decode(&repos)
	if err != nil {
		return nil, "", err
	}

	return repos.Repositories, resp.Header.Get("Link"), nil
}

func (r *Registry) getNextTags(baseUrl string, next string) ([]string, string, error) {
	url := fmt.Sprintf("%v%v", baseUrl, next)
	resp, err := r.getWithAuth(url)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	var tags tagsResponse
	decoder := json.NewDecoder(resp.Body)
	err = decoder.Decode(&tags)
	if err != nil {
		return nil, "", err
	}

	return tags.Tags, resp.Header.Get("Link"), nil
}
