package main

import (
	"database/sql"
	"errors"
	"fmt"
	"github.com/getsentry/raven-go"
	_ "github.com/lib/pq"
	"golang.org/x/crypto/sha3"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
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
	return sql.Open("postgres", "user=weiss dbname=weiss host=/run/postgresql")
}

func initializeDatabaseJustInCase() {
	db, err := getDatabase()
	if err != nil {
		raven.CaptureErrorAndWait(err, nil)
		log.Panic(err)
	}
	defer db.Close()
	_, qerr := db.Query("CREATE TABLE IF NOT EXISTS files (id CHARACTER VARYING PRIMARY KEY UNIQUE, hash TEXT UNIQUE, uploader TEXT, uploaded_when TIMESTAMP WITH TIME ZONE DEFAULT NOW())")
	if qerr != nil {
		raven.CaptureErrorAndWait(qerr, nil)
		log.Panic(qerr)
	}
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	file, header, ferr := r.FormFile("file")
	ext := path.Ext(header.Filename)
	if ferr != nil {
		raven.CaptureError(ferr, nil)
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, "Internal server error during file unfolding")
		return
	}
	defer file.Close()
	contents, cerr := ioutil.ReadAll(file)
	if cerr != nil {
		raven.CaptureError(cerr, nil)
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, "Internal server error during file unfolding")
		return
	}
	hash := fmt.Sprintf("%x", sha3.Sum512(contents))
	fmt.Printf("%#v\n", hash)
	user, _, _ := r.BasicAuth()
	db, derr := getDatabase()
	if derr != nil {
		raven.CaptureError(derr, nil)
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, "Internal server error during database connection")
		return
	}
	defer db.Close()
	existing, eqerr := db.Query("SELECT id FROM files WHERE hash = $1", hash)
	if eqerr != nil {
		raven.CaptureError(eqerr, nil)
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, "Internal server error during database check")
		return
	}
	defer existing.Close()
	for existing.Next() {
		var target_id string
		serr := existing.Scan(&target_id)
		if serr != nil {
			raven.CaptureError(serr, nil)
			w.WriteHeader(http.StatusInternalServerError)
			io.WriteString(w, "Internal server error during database fetch")
			return
		}
		http.Redirect(w, r, path.Join("/", "f", target_id+ext), http.StatusFound)
		return
	}
	var length = 1
	for length <= len(hash) {
		_, qerr := db.Query("INSERT INTO files (id, hash, uploader) VALUES ($1, $2, $3)", hash[:length], hash, user)
		if qerr == nil {
			break
		}
		length++
	}
	if length > len(hash) {
		raven.CaptureError(errors.New("hash already there"), nil)
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, "Internal server error during hash assignment")
		return
	}
	out, oerr := os.Create(path.Join(getWarehouse(), hash[:length]+ext))
	if oerr != nil {
		raven.CaptureError(oerr, nil)
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, "Internal server error during file creation")
		return
	}
	defer out.Close()
	_, werr := out.Write(contents)
	if werr != nil {
		raven.CaptureError(werr, nil)
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, "Internal server error during file writing")
		return
	}
	http.Redirect(w, r, path.Join("/", "f", hash[:length]+ext), http.StatusFound)
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
