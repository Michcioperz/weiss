package main

import (
	"github.com/getsentry/raven-go"
	"fmt"
	"log"
	"errors"
	"net/http"
	"os"
	"path"
	"io"
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
	env, yes = os.LookupEnv("VIRTUALENV")
	if yes {
		return env
	}
	env, _ = os.Getwd()
	return env
}

func getDatabase() (*sql.DB, error) {
	return sql.Open("postgres", "user=weiss dbname=weiss")
}

func initializeDatabaseJustInCase() {
	db, err := getDatabase()
	if err != nil {
		raven.CaptureErrorAndWait(err, nil)
		log.Panic(err)
	}
	defer db.Close()
	_, qerr := db.Query("CREATE TABLE IF NOT EXISTS files (id CHARACTER VARYING(64) PRIMARY KEY UNIQUE, hash CHARACTER(64) UNIQUE, uploader TEXT, uploaded_when TIMESTAMP WITH TIME ZONE DEFAULT NOW())")
	if qerr != nil {
		raven.CaptureErrorAndWait(qerr, nil)
		log.Panic(qerr)
	}
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	file, header, ferr := r.FormFile("file")
	defer file.Close()
	if ferr != nil {
		raven.CaptureError(ferr, nil)
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, "Internal server error during file unfolding")
		return
	}
	contents, cerr := ioutil.ReadAll(file)
	if cerr != nil {
		raven.CaptureError(cerr, nil)
    w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, "Internal server error during file unfolding")
		return
	}
	hash := fmt.Sprintf("%x", sha3.Sum512(contents))
	user, _, _ := r.BasicAuth()
	db, derr := getDatabase()
	defer db.Close()
	if derr != nil {
		raven.CaptureError(derr, nil)
    w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, "Internal server error during database connection")
		return
	}
	var length = 1
	for length <= 64 {
		_, qerr := db.Query("INSERT INTO files (id, hash, uploader) VALUES ($1, $2, $3)", hash[:length], hash, user)
		if qerr == nil {
			break
		}
		length++
	}
	if length > 64 {
		raven.CaptureError(errors.New("hash already there"), nil)
    w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, "Internal server error during hash assignment")
		return
	}
	ext := path.Ext(header.Filename)
	out, oerr := os.Create(path.Join(getWarehouse(), hash[:length] + ext))
	defer out.Close()
	if oerr != nil {
		raven.CaptureError(oerr, nil)
    w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, "Internal server error during file creation")
		return
	}
	_, werr := out.Write(contents)
	if werr != nil {
		raven.CaptureError(werr, nil)
    w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, "Internal server error during file writing")
		return
	}
	http.Redirect(w, r, path.Join("/", "f", hash[:length] + ext), http.StatusFound)
	return
}

func masterRelease() string {
	data, err := ioutil.ReadFile(path.Join(".git", "refs", "heads", "master"))
	if err != nil {
		return string(data)
	}
	return "unidentified"
}

func main() {
	raven.SetRelease(masterRelease())
	initializeDatabaseJustInCase()
	http.HandleFunc("/u", raven.RecoveryHandler(uploadHandler))
	http.ListenAndServe(":9009", nil)
}
