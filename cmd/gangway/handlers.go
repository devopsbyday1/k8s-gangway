// Copyright © 2017 Heptio
// Copyright © 2017 Craig Tracey
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	htmltemplate "html/template"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	texttemplate "text/template"

	log "github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api/v1"
	"sigs.k8s.io/yaml"
)

// userInfo stores information about an authenticated user
type userInfo struct {
	ClusterName  string
	Username     string
	Claims       map[string]interface{}
	KubeCfgUser  string
	IDToken      string
	RefreshToken string
	ClientID     string
	ClientSecret string
	TokenURL     string
	IssuerURL    string
	APIServerURL string
	ClusterCA    string
	HTTPPath     string
}

// homeInfo is used to store dynamic properties on
type homeInfo struct {
	HTTPPath string
}

func serveTemplate(tmplFile string, data interface{}, w http.ResponseWriter) {
	var (
		templateData []byte
		err          error
	)

	// Use custom templates if provided, otherwise use embedded templates
	if cfg.CustomHTMLTemplatesDir != "" {
		templateData, err = os.ReadFile(fmt.Sprintf("%s/%s", cfg.CustomHTMLTemplatesDir, tmplFile))
	} else {
		templateData, err = templateFS.ReadFile("templates/" + tmplFile)
	}

	if err != nil {
		log.Errorf("Failed to find template asset: %s", tmplFile)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	tmpl := htmltemplate.New(tmplFile)
	tmpl, err = tmpl.Parse(string(templateData))
	if err != nil {
		log.Errorf("Failed to parse template: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tmpl.ExecuteTemplate(w, tmplFile, data); err != nil {
		log.Errorf("Failed to execute template %s: %v", tmplFile, err)
	}
}

func generateKubeConfig(cfg *userInfo) clientcmdapi.Config {
	// Exec credential plugin: calls the gangway-generated helper script via sh.
	// The script stores the refresh_token and silently renews the id_token on
	// expiry — replicating the old auth-provider: oidc set-and-forget behaviour.
	// Only requires curl + base64 (no extra tools to install).
	kcfg := clientcmdapi.Config{
		Kind:           "Config",
		APIVersion:     "v1",
		CurrentContext: cfg.ClusterName,
		Clusters: []clientcmdapi.NamedCluster{
			{
				Name: cfg.ClusterName,
				Cluster: clientcmdapi.Cluster{
					Server:                   cfg.APIServerURL,
					CertificateAuthorityData: []byte(cfg.ClusterCA),
				},
			},
		},
		Contexts: []clientcmdapi.NamedContext{
			{
				Name: cfg.ClusterName,
				Context: clientcmdapi.Context{
					Cluster:  cfg.ClusterName,
					AuthInfo: cfg.KubeCfgUser,
				},
			},
		},
		AuthInfos: []clientcmdapi.NamedAuthInfo{
			{
				Name: cfg.KubeCfgUser,
				AuthInfo: clientcmdapi.AuthInfo{
					Exec: &clientcmdapi.ExecConfig{
						APIVersion:      "client.authentication.k8s.io/v1",
						Command:         "/bin/sh",
						Args:            []string{"-c", fmt.Sprintf("exec $HOME/.kube/gangway-%s.sh", cfg.ClusterName)},
						InteractiveMode: clientcmdapi.NeverExecInteractiveMode,
					},
				},
			},
		},
	}
	return kcfg
}

// serveScriptTemplate renders a text/template (no HTML escaping) as a
// downloadable file. Used for the generated shell script helper.
func serveScriptTemplate(tmplFile string, data interface{}, filename string, w http.ResponseWriter) {
	templateData, err := templateFS.ReadFile("templates/" + tmplFile)
	if err != nil {
		log.Errorf("Failed to find template asset: %s", tmplFile)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	tmpl, err := texttemplate.New(tmplFile).Parse(string(templateData))
	if err != nil {
		log.Errorf("Failed to parse script template: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/x-sh")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	if err := tmpl.ExecuteTemplate(w, tmplFile, data); err != nil {
		log.Errorf("Failed to execute script template %s: %v", tmplFile, err)
	}
}

// helperScriptHandler serves the per-user token helper shell script.
// The script embeds the user's current tokens and handles silent refresh.
func helperScriptHandler(w http.ResponseWriter, r *http.Request) {
	info := generateInfo(w, r)
	if info == nil {
		return
	}
	filename := fmt.Sprintf("gangway-%s.sh", info.ClusterName)
	serveScriptTemplate("helper.sh.tmpl", info, filename, w)
}

func loginRequired(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, err := gangwayUserSession.Session.Get(r, "gangway_id_token")
		if err != nil {
			http.Redirect(w, r, cfg.GetRootPathPrefix(), http.StatusTemporaryRedirect)
			return
		}

		if session.Values["id_token"] == nil {
			http.Redirect(w, r, cfg.GetRootPathPrefix(), http.StatusTemporaryRedirect)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	data := &homeInfo{
		HTTPPath: cfg.HTTPPath,
	}

	serveTemplate("home.tmpl", data, w)
}

func loginHandler(w http.ResponseWriter, r *http.Request) {

	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("Failed to generate random state: %v", err)
	}
	state := url.QueryEscape(base64.StdEncoding.EncodeToString(b))

	session, err := gangwayUserSession.Session.Get(r, "gangway")
	if err != nil {
		log.Errorf("Got an error in login: %s", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	session.Values["state"] = state
	err = session.Save(r, w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	audience := oauth2.SetAuthURLParam("audience", cfg.Audience)
	url := oauth2Cfg.AuthCodeURL(state, audience)

	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	gangwayUserSession.Cleanup(w, r, "gangway")
	gangwayUserSession.Cleanup(w, r, "gangway_id_token")
	gangwayUserSession.Cleanup(w, r, "gangway_refresh_token")
	http.Redirect(w, r, cfg.GetRootPathPrefix(), http.StatusTemporaryRedirect)
}

func callbackHandler(w http.ResponseWriter, r *http.Request) {
	ctx := context.WithValue(r.Context(), oauth2.HTTPClient, transportConfig.HTTPClient)

	// load up session cookies
	session, err := gangwayUserSession.Session.Get(r, "gangway")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sessionIDToken, err := gangwayUserSession.Session.Get(r, "gangway_id_token")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sessionRefreshToken, err := gangwayUserSession.Session.Get(r, "gangway_refresh_token")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// verify the state string
	state := r.URL.Query().Get("state")

	if state != session.Values["state"] {
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return
	}

	// use the access code to retrieve a token
	code := r.URL.Query().Get("code")
	token, err := o2token.Exchange(ctx, code)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sessionIDToken.Values["id_token"] = token.Extra("id_token")
	sessionRefreshToken.Values["refresh_token"] = token.RefreshToken

	// save the session cookies
	err = session.Save(r, w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = sessionIDToken.Save(r, w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = sessionRefreshToken.Save(r, w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("%s/commandline", cfg.HTTPPath), http.StatusSeeOther)
}

func commandlineHandler(w http.ResponseWriter, r *http.Request) {
	info := generateInfo(w, r)
	if info == nil {
		// generateInfo writes to the ResponseWriter if it encounters an error.
		// TODO(abrand): Refactor this.
		return
	}

	serveTemplate("commandline.tmpl", info, w)
}

func kubeConfigHandler(w http.ResponseWriter, r *http.Request) {
	info := generateInfo(w, r)
	if info == nil {
		// generateInfo writes to the ResponseWriter if it encounters an error.
		// TODO(abrand): Refactor this.
		return
	}

	d, err := yaml.Marshal(generateKubeConfig(info))
	if err != nil {
		log.Errorf("Error creating kubeconfig - %s", err.Error())
		http.Error(w, "Error creating kubeconfig", http.StatusInternalServerError)
		return
	}

	// tell the browser the returned content should be downloaded
	w.Header().Add("Content-Disposition", "Attachment")
	if _, err := w.Write(d); err != nil {
		log.Errorf("Error writing kubeconfig response: %v", err)
	}
}

func generateInfo(w http.ResponseWriter, r *http.Request) *userInfo {
	// read in public ca.crt to output in commandline copy/paste commands
	var caBytes []byte
	file, err := os.Open(cfg.ClusterCAPath)
	if err != nil {
		// let us know that we couldn't open the file. This only causes missing output,
		// does not impact actual function of program
		log.Errorf("Failed to open CA file. %s", err)
	} else {
		defer file.Close()
		caBytes, err = io.ReadAll(file)
		if err != nil {
			log.Warningf("Could not read CA file: %s", err)
		}
	}

	// load the session cookies
	sessionIDToken, err := gangwayUserSession.Session.Get(r, "gangway_id_token")
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return nil
	}
	sessionRefreshToken, err := gangwayUserSession.Session.Get(r, "gangway_refresh_token")
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return nil
	}

	idTokenStr, ok := sessionIDToken.Values["id_token"].(string)
	if !ok {
		gangwayUserSession.Cleanup(w, r, "gangway")
		gangwayUserSession.Cleanup(w, r, "gangway_id_token")
		gangwayUserSession.Cleanup(w, r, "gangway_refresh_token")

		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return nil
	}

	refreshToken, ok := sessionRefreshToken.Values["refresh_token"].(string)
	if !ok {
		gangwayUserSession.Cleanup(w, r, "gangway")
		gangwayUserSession.Cleanup(w, r, "gangway_id_token")
		gangwayUserSession.Cleanup(w, r, "gangway_refresh_token")

		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return nil
	}

	// Verify and parse the ID token
	idToken, err := idTokenVerifier.Verify(r.Context(), idTokenStr)
	if err != nil {
		http.Error(w, "Could not verify ID token", http.StatusInternalServerError)
		return nil
	}

	var claims map[string]interface{}
	if err := idToken.Claims(&claims); err != nil {
		http.Error(w, "Could not extract token claims", http.StatusInternalServerError)
		return nil
	}

	username, ok := claims[cfg.UsernameClaim].(string)
	if !ok {
		http.Error(w, "Could not parse Username claim", http.StatusInternalServerError)
		return nil
	}

	kubeCfgUser := strings.Join([]string{username, cfg.ClusterName}, "@")

	if cfg.EmailClaim != "" {
		log.Warn("using the Email Claim config setting is deprecated. Gangway uses `UsernameClaim@ClusterName`. This field will be removed in a future version.")
	}

	issuerURL := idToken.Issuer()

	if cfg.ClientSecret == "" {
		log.Warn("Setting an empty Client Secret should only be done if you have no other option and is an inherent security risk.")
	}

	info := &userInfo{
		ClusterName:  cfg.ClusterName,
		Username:     username,
		Claims:       claims,
		KubeCfgUser:  kubeCfgUser,
		IDToken:      idTokenStr,
		RefreshToken: refreshToken,
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		TokenURL:     cfg.TokenURL,
		IssuerURL:    issuerURL,
		APIServerURL: cfg.APIServerURL,
		ClusterCA:    string(caBytes),
		HTTPPath:     cfg.HTTPPath,
	}
	return info
}
