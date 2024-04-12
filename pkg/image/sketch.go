package image

import (
	"fmt"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/tomekjarosik/geranos/pkg/image/duplicator"
	"github.com/tomekjarosik/geranos/pkg/image/filesegment"
	"os"
	"path/filepath"
)

type SketchConstructor interface {
	Construct(dir string, fileRecipes []*fileRecipe) (Statistics, error)
}

func NewSketchConstructor(rootDir string) SketchConstructor {
	return &defaultSketchConstructor{rootDirectory: rootDir}
}

type defaultSketchConstructor struct {
	rootDirectory string

	stats Statistics
}

type cloneCandidate struct {
	descriptors []*v1.Descriptor
	dirPath     string
}

func resizeFile(filePath string, newSize int64) error {
	// Open file with read and write permissions
	file, err := os.OpenFile(filePath, os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	// Resize file to the specified newSize
	err = file.Truncate(newSize)
	if err != nil {
		return err
	}

	return nil
}

func (sc *defaultSketchConstructor) Construct(dir string, fileRecipes []*fileRecipe) (Statistics, error) {
	cloneCandidates, err := sc.findCloneCandidates()
	if err != nil {
		return Statistics{}, fmt.Errorf("encountered error while looking for manifests: %w", err)
	}
	err = os.MkdirAll(dir, 0o755)
	if err != nil {
		return Statistics{}, fmt.Errorf("unable to create directory '%v': %w", dir, err)
	}

	for _, fr := range fileRecipes {
		// we will process each FR exactly once
		// fr can easily have 1000 layers,
		// each manifest can also have more than 1000 layers
		// we need to compute best score in expected linear time
		segmentsDigestMap := make(map[string]*filesegment.Descriptor)
		for _, seg := range fr.Segments {
			segmentsDigestMap[seg.Digest().String()] = seg
		}
		bestScore := 0
		var bestCloneCandidate *cloneCandidate
		for _, cc := range cloneCandidates {
			score := sc.computeScore(segmentsDigestMap, cc)
			if score > bestScore {
				bestScore = score
				bestCloneCandidate = cc
			}
		}
		if bestCloneCandidate == nil {
			continue
		}
		sc.stats.BytesClonedCount += fr.Size()
		sc.stats.MatchingSegmentsCount += int64(bestScore)
		src := filepath.Join(bestCloneCandidate.dirPath, fr.Filename)
		dest := filepath.Join(dir, fr.Filename)
		if src == dest {
			continue
		}
		err = duplicator.CloneFile(src, dest)
		if err != nil {
			return sc.stats, fmt.Errorf("unable to clone source file '%v' to destination '%v': %w", src, dest, err)
		}
		err = resizeFile(dest, fr.Size())
		if err != nil {
			return sc.stats, fmt.Errorf("error occured while resizing file '%v' to its new size '%v': %w", dest, fr.Size(), err)
		}
	}
	return sc.stats, nil
}

// parseManifestFile represents a placeholder for your actual parsing logic.
func (sc *defaultSketchConstructor) findCloneCandidates() ([]*cloneCandidate, error) {
	type Job struct {
		path string
	}
	jobs := make(chan Job, 2)

	go func() {
		filepath.Walk(sc.rootDirectory, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return fmt.Errorf("error accessing path %q: %w\n", path, err)
			}
			if !info.IsDir() && info.Name() == LocalManifestFilename {
				jobs <- Job{path: path}
			}
			return nil
		})
		close(jobs) // Close the jobs channel when done walking the directory
	}()

	candidates := make([]*cloneCandidate, 0)
	for job := range jobs {
		f, err := os.Open(job.path)
		if err != nil {
			return nil, err
		}
		manifest, err := v1.ParseManifest(f)
		if err == nil {
			descriptors := make([]*v1.Descriptor, 0)
			for _, d := range manifest.Layers {
				descriptors = append(descriptors, &d)
			}
			candidates = append(candidates, &cloneCandidate{
				descriptors: descriptors,
				dirPath:     filepath.Dir(job.path),
			})
		}
		f.Close()
		if err != nil {
			return nil, err
		}
	}
	return candidates, nil
}

func (sc *defaultSketchConstructor) computeScore(segmentDigestMap map[string]*filesegment.Descriptor, m *cloneCandidate) int {
	score := 0
	for _, descriptor := range m.descriptors {
		_, ok := segmentDigestMap[descriptor.Digest.String()]
		if ok {
			score += 1
		}
	}

	return score
}
