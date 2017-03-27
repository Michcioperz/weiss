package main

import (
	"github.com/getsentry/raven-go"
	"fmt"
	"net/http"
	"os/exec"
	"os"
	"io"
	"path"
	"io/ioutil"
	"database/sql"
	"golang.org/x/crypto/sha3"
	_ "github.com/lib/pq"
)

func getWarehouse() string {
	env, yes := os.LookupEnv("WAREHOUSE")
	if yes {
		return env
	}
	env, yes := os.LookupEnv("VIRTUALENV")
	if yes {
		return env
	}
	env, yes := os.Getwd()
	return env
}

func getDatabase() {
	return sql.Open("postgres", "user=weiss dbname=weiss")
}

func initializeDatabaseJustInCase() {
	db, err := getDatabase()
	defer db.Close()
	db.Query("CREATE TABLE IF NOT EXISTS files (id CHARACTER VARYING(64) PRIMARY KEY UNIQUE, hash CHARACTER(64) UNIQUE, uploader TEXT, uploaded_when TIMESTAMP WITH TIME ZONE DEFAULT NOW())")
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	file, header, err := r.FormFile("file")
	defer file.close()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write("Internal server error during file unfolding")
		return
	}
	var contents, err := ioutil.ReadAll(file)
	if err != nil {
    w.WriteHeader(http.StatusInternalServerError)
		w.Write("Internal server error during file unfolding")
		return
	}
	hash = fmt.Sprintf("%x", sha3.Sum512(contents))
	user, _, _ := r.BasicAuth()
	db, err := getDatabase()
	defer db.Close()
	if err != nil {
    w.WriteHeader(http.StatusInternalServerError)
		w.Write("Internal server error during database connection")
		return
	}
	length := 1
	while length <= 64 {
		_, err := db.Query("INSERT INTO files (id, hash, uploader) VALUES ($1, $2, $3)", hash[:length], hash, user)
		if err == nil {
			break
		}
		length++
	}
	if length > 64 {
    w.WriteHeader(http.StatusInternalServerError)
		w.Write("Internal server error during hash assignment")
		return
	}
	out, err := os.Create(path.Join(getWarehouse(), hash[:length] + path.Ext(header.Filename)))
	defer out.Close()
	if err != nil {
    w.WriteHeader(http.StatusInternalServerError)
		w.Write("Internal server error during file creation")
		return
	}
	_, err = out.Write(contents)
	if err != nil {
    w.WriteHeader(http.StatusInternalServerError)
		w.Write("Internal server error during file writing")
		return
	}
	http.Redirect(w, r, path.Join("/", "f", hash[:length] + ext), http.StatusFound)
	return
}

func main() {
	initializeDatabaseJustInCase()
	http.HandleFunc("/u", uploadHandler)
	http.ListenAndServe(":9009", nil)
}
