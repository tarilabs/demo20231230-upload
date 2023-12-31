package server

import (
	"demo20231230-upload/internal/openapi"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	oras "oras.land/oras-go/v2"
	orasfile "oras.land/oras-go/v2/content/file"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/retry"
)

type ServerImpl struct{}

func (*ServerImpl) PostUploadOras(w http.ResponseWriter, r *http.Request) {
	// var myValue TType1
	// if err := binding.MultipartForm(r, &myValue); err != nil { // NOT working for file fields
	// 	panic(err)
	// }
	// fmt.Println(myValue)

	err := r.ParseMultipartForm(32 << 20) // maxMemory 32MB
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var1 := r.FormValue("var1")

	_, h, err := r.FormFile("fileName")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	log.Println(h.Filename)

	file, err := h.Open()
	checkErr(err)
	data, err := io.ReadAll(file)
	checkErr(err)

	dname, err := os.MkdirTemp("", "sampledir")
	checkErr(err)
	log.Println("dname for temporary local storage (might in-memory later): ", dname)

	f, err := os.CreateTemp(dname, h.Filename)
	checkErr(err)
	log.Println("file for temporary local storage (might in-memory later): ", f)
	_, err = f.Write(data)
	checkErr(err)
	err = f.Close()
	checkErr(err)

	fs, err := orasfile.New(dname)
	checkErr(err)
	defer fs.Close()

	mediaType := "application/vnd.test.file"
	fileNames := []string{f.Name()}
	fileDescriptors := make([]v1.Descriptor, 0, len(fileNames))
	for _, name := range fileNames {
		fileDescriptor, err := fs.Add(r.Context(), name, mediaType, "")
		checkErr(err)
		fileDescriptors = append(fileDescriptors, fileDescriptor)
		log.Printf("file descriptor for %s: %v\n", name, fileDescriptor)
	}

	artifactType := "application/vnd.test.artifact"
	opts := oras.PackManifestOptions{
		Layers: fileDescriptors,
	}
	manifestDescriptor, err := oras.PackManifest(r.Context(), fs, oras.PackManifestVersion1_1_RC4, artifactType, opts)
	checkErr(err)
	log.Println("manifest descriptor:", manifestDescriptor)

	tag := var1
	if err = fs.Tag(r.Context(), manifestDescriptor, tag); err != nil {
		log.Println("Error: ", err)
	}

	reg := os.Getenv("REGISTRY")
	repo, err := remote.NewRepository(reg + "/mmortari/orastest")
	checkErr(err)
	repo.Client = &auth.Client{
		Client: retry.DefaultClient,
		Cache:  auth.DefaultCache,
		Credential: auth.StaticCredential(reg, auth.Credential{
			Username: os.Getenv("USERNAME"),
			Password: os.Getenv("PASSWORD"),
		}),
	}

	_, err = oras.Copy(r.Context(), fs, tag, repo, tag, oras.DefaultCopyOptions)
	checkErr(err)

	resp := make(map[string]string)
	resp["message"] = "Done uploading with ORAS"
	jsonResp, err := json.Marshal(resp)
	if err != nil {
		log.Fatalf("error handling JSON marshal. Err: %v", err)
	}

	_, _ = w.Write(jsonResp)
}

func checkErr(e error) {
	if e != nil {
		log.Printf("error: %v", e)
	}
}

func (s *Server) RegisterRoutes() http.Handler {
	var myRoutesApi ServerImpl

	r := chi.NewRouter()
	r.Use(middleware.Logger)

	r.Get("/hello", s.HelloWorldHandler)

	r.Mount("/", openapi.Handler(&myRoutesApi))

	r.Handle("/swagger/*", http.StripPrefix("/swagger/", http.FileServer(http.Dir("dist"))))
	r.Handle("/openapi.yaml", http.FileServer(http.Dir("api")))

	log.Println("Registered routes")
	return r
}

func (s *Server) HelloWorldHandler(w http.ResponseWriter, r *http.Request) {
	resp := make(map[string]string)
	resp["message"] = "Hello World"

	jsonResp, err := json.Marshal(resp)
	if err != nil {
		log.Fatalf("error handling JSON marshal. Err: %v", err)
	}

	_, _ = w.Write(jsonResp)
}
