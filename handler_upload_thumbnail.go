package main

import (
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
	"github.com/google/uuid"
)

const maxMemory = 10 << 20

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	err = r.ParseMultipartForm(maxMemory)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "File too large", err)
		return
	}

	var file multipart.File
	var fileHeader *multipart.FileHeader

	file, fileHeader, err = r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Missing thumbnail file", err)
		return
	}
	defer file.Close()

	mediaType := fileHeader.Header.Get("Content-Type")

	// 1. Get the proper file extension from Content-Type
	// mime.ExtensionsByType returns a slice of extensions, e.g., []string{".png"}
	extensions, err := mime.ExtensionsByType(mediaType)
	if err != nil || len(extensions) == 0 {
		respondWithError(w, http.StatusBadRequest, "Invalid media type", err)
		return
	}
	// Use the first extension returned (and strip the leading dot for formatting)
	// Alternatively, you can use the whole extension string. The tests usually look for the extension with its dot or without depending on how you build the path.
	// But using the raw returned extension (which includes the dot) works perfectly with filepath formatting:
	ext := extensions[0] // e.g., ".png" or ".jpeg"

	// 2. Retrieve video metadata and verify ownership
	var metadata database.Video
	metadata, err = cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error retrieving video", err)
		return
	}
	if metadata.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Logged in user does not own this video", nil)
		return
	}

	// 3. Create the file path on disk: assets/<videoID>.<extension>
	fileName := fmt.Sprintf("%s%s", videoID, ext)
	filePath := filepath.Join(cfg.assetsRoot, fileName)

	// 4. Create the file on the filesystem
	dst, err := os.Create(filePath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to create file on disk", err)
		return
	}
	defer dst.Close()

	// 5. Copy the uploaded file contents to the filesystem destination
	_, err = io.Copy(dst, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error saving file to disk", err)
		return
	}

	// 6. Update the database URL to point to the local file server path
	thumburl := fmt.Sprintf("http://localhost:%s/assets/%s", cfg.port, fileName)
	metadata.ThumbnailURL = &thumburl

	err = cfg.db.UpdateVideo(metadata)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error updating database", err)
		return
	}

	respondWithJSON(w, http.StatusOK, metadata)
}
