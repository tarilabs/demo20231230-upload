package server

import (
	"context"
	"demo20231230-upload/internal/openapi"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	oras "oras.land/oras-go/v2"
	orasfile "oras.land/oras-go/v2/content/file"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/retry"
)

type ServerImpl struct{}

type S3Config struct {
	AWS_ACCESS_KEY_ID     string
	AWS_DEFAULT_REGION    string
	AWS_S3_BUCKET         string
	AWS_S3_ENDPOINT       string
	AWS_SECRET_ACCESS_KEY string
}

func getS3Config(ctx context.Context, secretName string) (*S3Config, error) {
	var namespace string
	config, err := rest.InClusterConfig()
	if err != nil {
		kubeconfig :=
			clientcmd.NewDefaultClientConfigLoadingRules().GetDefaultFilename()
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		// namespace if out-of-cluster
		clientCfg, _ := clientcmd.NewDefaultClientConfigLoadingRules().Load()
		namespace = clientCfg.Contexts[clientCfg.CurrentContext].Namespace
		log.Println("out-of-cluster namespace from clientCfg Context:", namespace)
		if namespace == "" {
			log.Println("since empty, setting default namespace to 'default'", namespace)
			namespace = "default"
		}
		if err != nil {
			return nil, err
		}
	} else {
		// TODO 'POD_NAMESPACE' for in-cluster? https://stackoverflow.com/a/61534815/893991
		namespace = os.Getenv("POD_NAMESPACE")
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	log.Println("Using namespace:", namespace)

	secret, err := client.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		panic(err)
	}
	result := S3Config{
		AWS_ACCESS_KEY_ID:     string(secret.Data["AWS_ACCESS_KEY_ID"]),
		AWS_DEFAULT_REGION:    string(secret.Data["AWS_DEFAULT_REGION"]),
		AWS_S3_BUCKET:         string(secret.Data["AWS_S3_BUCKET"]),
		AWS_S3_ENDPOINT:       string(secret.Data["AWS_S3_ENDPOINT"]),
		AWS_SECRET_ACCESS_KEY: string(secret.Data["AWS_SECRET_ACCESS_KEY"]),
	}
	return &result, nil
}

func (*ServerImpl) PostUploadS3(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(32 << 20) // maxMemory 32MB
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var1 := r.FormValue("var1")

	s3config, err := getS3Config(r.Context(), var1)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	log.Printf("s3config: %v", s3config)

	endpoint := s3config.AWS_S3_ENDPOINT
	// TODO: strip if starting with 'http://' or 'https://'
	accessKeyID := s3config.AWS_ACCESS_KEY_ID
	secretAccessKey := s3config.AWS_SECRET_ACCESS_KEY
	// TODO: revise useSSL := false
	useSSL := false
	bucketName := s3config.AWS_S3_BUCKET
	_ = s3config.AWS_DEFAULT_REGION // used only for bucket-creation

	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		log.Fatalln(err)
	}

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

	objectName := h.Filename
	filePath := f.Name()
	contentType := "application/octet-stream"

	info, err := minioClient.FPutObject(r.Context(), bucketName, objectName, filePath, minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		log.Fatalln(err)
	}
	log.Printf("Successfully uploaded %s of size %d\n", objectName, info.Size)

	resp := make(map[string]string)
	resp["message"] = "Done uploading with S3"
	jsonResp, err := json.Marshal(resp)
	if err != nil {
		log.Fatalf("error handling JSON marshal. Err: %v", err)
	}

	_, _ = w.Write(jsonResp)
}

func (*ServerImpl) PostUploadOras(w http.ResponseWriter, r *http.Request) {
	// var myValue TType1
	// if err := binding.MultipartForm(r, &myValue); err != nil { // TODO NOT working for file fields
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
