package llm

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"

	"arkloop/services/worker/internal/imageutil"
)

type preparedModelInputImage struct {
	MimeType         string
	Data             []byte
	Base64           string
	DataURL          string
	Base64ImageBytes int
	Err              error
}

const modelInputImageCacheMaxEntries = 128

var modelInputImageCache = struct {
	sync.Mutex
	order []string
	items map[string]preparedModelInputImage
}{
	items: map[string]preparedModelInputImage{},
}

type modelInputImageJob struct {
	MessageIndex int
	PartIndex    int
	Part         ContentPart
	Prepared     preparedModelInputImage
}

func PrepareRequestModelInputImages(req *Request) {
	if req == nil || req.modelInputImagesPrepared {
		return
	}

	jobs := make([]modelInputImageJob, 0)
	for msgIndex, msg := range req.Messages {
		for partIndex, part := range msg.Content {
			if part.Kind() != "image" {
				continue
			}
			jobs = append(jobs, modelInputImageJob{
				MessageIndex: msgIndex,
				PartIndex:    partIndex,
				Part:         part,
			})
		}
	}
	if len(jobs) > 0 {
		prepareModelInputImageJobs(jobs)
		messages := make([]Message, len(req.Messages))
		copy(messages, req.Messages)
		partsByMessage := make([][]ContentPart, len(req.Messages))
		for i := range jobs {
			job := &jobs[i]
			parts := partsByMessage[job.MessageIndex]
			if parts == nil {
				parts = make([]ContentPart, len(req.Messages[job.MessageIndex].Content))
				copy(parts, req.Messages[job.MessageIndex].Content)
				partsByMessage[job.MessageIndex] = parts
				messages[job.MessageIndex].Content = parts
			}
			parts[job.PartIndex].modelInputImage = &job.Prepared
		}
		req.Messages = messages
	}
	req.modelInputImagesPrepared = true
}

func prepareModelInputImageJobs(jobs []modelInputImageJob) {
	if len(jobs) == 1 {
		jobs[0].Prepared = prepareModelInputImageCached(jobs[0].Part)
		return
	}

	var wg sync.WaitGroup
	wg.Add(len(jobs))
	for i := range jobs {
		go func(index int) {
			defer wg.Done()
			jobs[index].Prepared = prepareModelInputImageCached(jobs[index].Part)
		}(i)
	}
	wg.Wait()
}

func modelInputImage(part ContentPart) (string, []byte, error) {
	prepared := modelInputImagePayload(part)
	if prepared.Err != nil {
		return "", nil, prepared.Err
	}
	return prepared.MimeType, prepared.Data, nil
}

func modelInputImagePayload(part ContentPart) preparedModelInputImage {
	if part.modelInputImage != nil {
		return *part.modelInputImage
	}
	return prepareModelInputImageCached(part)
}

func prepareModelInputImageCached(part ContentPart) preparedModelInputImage {
	cacheKey, ok := modelInputImageCacheKey(part)
	if ok {
		if prepared, found := lookupPreparedModelInputImage(cacheKey); found {
			return prepared
		}
	}
	prepared := prepareModelInputImage(part)
	if ok && prepared.Err == nil {
		storePreparedModelInputImage(cacheKey, prepared)
	}
	return prepared
}

func modelInputImageCacheKey(part ContentPart) (string, bool) {
	if part.Attachment == nil || len(part.Data) == 0 {
		return "", false
	}
	sum := sha256.Sum256(part.Data)
	return strings.Join([]string{
		strings.TrimSpace(part.Attachment.Key),
		strings.TrimSpace(part.Attachment.MimeType),
		fmt.Sprintf("%d", len(part.Data)),
		fmt.Sprintf("%x", sum[:]),
	}, "\x00"), true
}

func lookupPreparedModelInputImage(cacheKey string) (preparedModelInputImage, bool) {
	modelInputImageCache.Lock()
	defer modelInputImageCache.Unlock()
	prepared, ok := modelInputImageCache.items[cacheKey]
	if !ok {
		return preparedModelInputImage{}, false
	}
	return clonePreparedModelInputImage(prepared), true
}

func storePreparedModelInputImage(cacheKey string, prepared preparedModelInputImage) {
	modelInputImageCache.Lock()
	defer modelInputImageCache.Unlock()
	if _, exists := modelInputImageCache.items[cacheKey]; !exists {
		modelInputImageCache.order = append(modelInputImageCache.order, cacheKey)
	}
	modelInputImageCache.items[cacheKey] = clonePreparedModelInputImage(prepared)
	for len(modelInputImageCache.order) > modelInputImageCacheMaxEntries {
		evictKey := modelInputImageCache.order[0]
		copy(modelInputImageCache.order, modelInputImageCache.order[1:])
		modelInputImageCache.order = modelInputImageCache.order[:len(modelInputImageCache.order)-1]
		delete(modelInputImageCache.items, evictKey)
	}
}

func clonePreparedModelInputImage(prepared preparedModelInputImage) preparedModelInputImage {
	if len(prepared.Data) > 0 {
		prepared.Data = append([]byte(nil), prepared.Data...)
	}
	return prepared
}

func prepareModelInputImage(part ContentPart) preparedModelInputImage {
	if part.Attachment == nil {
		return preparedModelInputImage{Err: fmt.Errorf("image attachment is required")}
	}
	if len(part.Data) == 0 {
		return preparedModelInputImage{Err: fmt.Errorf("image attachment data is required")}
	}

	mimeType := strings.TrimSpace(part.Attachment.MimeType)
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	data := part.Data
	if key := strings.TrimSpace(part.Attachment.Key); key != "" {
		data, mimeType = imageutil.PrepareModelInputImage(data, mimeType, key)
	}
	encoded := base64.StdEncoding.EncodeToString(data)
	return preparedModelInputImage{
		MimeType:         mimeType,
		Data:             data,
		Base64:           encoded,
		DataURL:          "data:" + mimeType + ";base64," + encoded,
		Base64ImageBytes: len(encoded),
	}
}

func modelInputImageDataURL(part ContentPart) (string, error) {
	prepared := modelInputImagePayload(part)
	if prepared.Err != nil {
		return "", prepared.Err
	}
	return prepared.DataURL, nil
}

func modelInputImageBase64(part ContentPart) (string, string, error) {
	prepared := modelInputImagePayload(part)
	if prepared.Err != nil {
		return "", "", prepared.Err
	}
	return prepared.MimeType, prepared.Base64, nil
}

func modelInputImageBase64Size(part ContentPart) (int, error) {
	prepared := modelInputImagePayload(part)
	if prepared.Err != nil {
		return 0, prepared.Err
	}
	return prepared.Base64ImageBytes, nil
}
