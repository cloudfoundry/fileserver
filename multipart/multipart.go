package multipart

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
)

// FILE UPLOAD HELPERS

func NewRequestFromReader(contentLength int64, body io.Reader, formField string, fileName string) (*http.Request, error) {
	pipeReader, pipeWriter := io.Pipe()

	multipartLength, multipartBoundary, err := computeMultipartFormLength(formField, fileName)
	if err != nil {
		return nil, err
	}

	multipartWriter := multipart.NewWriter(pipeWriter)
	multipartWriter.SetBoundary(multipartBoundary)
	go func() {
		var err error
		defer func() {
			pipeWriter.CloseWithError(err)
		}()

		filePartWriter, err := multipartWriter.CreateFormFile(formField, fileName)
		if err != nil {
			return
		}

		_, err = io.Copy(filePartWriter, body)
		if err != nil {
			return
		}

		err = multipartWriter.Close()
	}()

	uploadReq, err := http.NewRequest("POST", "", pipeReader)
	if err != nil {
		return nil, err
	}

	uploadReq.Header.Set("Content-Type", multipartWriter.FormDataContentType())
	uploadReq.ContentLength = contentLength + multipartLength

	return uploadReq, nil
}

//computes the length of the multi-part form request, minus the content of the form itself
func computeMultipartFormLength(formField string, fileName string) (int64, string, error) {
	multipartBuffer := &bytes.Buffer{}
	multipartWriter := multipart.NewWriter(multipartBuffer)
	_, err := multipartWriter.CreateFormFile(formField, fileName)
	multipartWriter.Close()

	return int64(multipartBuffer.Len()), multipartWriter.Boundary(), err
}
