package video

import (
	"errors"
	"io/ioutil"
	"strconv"
	"strings"
	"sync"

	"github.com/0x113/x-media/env"
	"github.com/anaskhan96/soup"
	log "github.com/sirupsen/logrus"
)

type VideoService interface {
	Save() error
	AllMovies() ([]*Movie, error)
	SaveTVShows() error
	AllTvSeries() ([]*TVSeries, error)
	TvSeriesEpisodes(title string) ([]*Season, error)
	MoviePath(title string) string
	MovieSubtitles(title string) (string, error)
	GetMovie(id string) (*Movie, error)
	GetTvSeries(id string) (*TVSeries, error)
}

type videoService struct {
	repo VideoRepository
}

func NewVideoService(repo VideoRepository) VideoService {
	return &videoService{
		repo,
	}
}

func (s *videoService) updateMovie(videoFileName string) error {
	movie, _, err := s.getMovieAndTvSeriesInfo(videoFileName) // returns *Movie, *TVSeries, error

	if err != nil {
		return err
	}

	if err := s.repo.SaveMovie(movie); err != nil {
		return err
	}

	log.Infof("Successfully updated movie [title=%s, file_name=%s]", movie.Title, videoFileName)
	return nil
}

func (s *videoService) updateTvSeries(tvSeriesDir string) error {
	_, tvSeries, err := s.getMovieAndTvSeriesInfo(tvSeriesDir)

	if err != nil {
		return err
	}

	if err := s.repo.SaveTvSeries(tvSeries); err != nil {
		return err
	}

	log.Infof("Successfully updated TV series [title=%s, tv_series_dir=%s]", tvSeries.Title, tvSeriesDir)
	return nil
}

func (s *videoService) Save() error {
	log.Infoln("Updating movie database...")

	// check if video dir path ends with slash
	videoDirPath := env.EnvString("VIDEO_DIR")
	if !strings.HasSuffix(videoDirPath, "/") {
		videoDirPath += "/"
	}
	videos, err := s.getVideos(videoDirPath)
	if err != nil {
		log.Errorln("Unable to get list of movies")
		return err
	}

	var wg sync.WaitGroup

	// get movies from db to check if any have been removed
	moviesInDb, err := s.repo.FindAllMovies()
	if err != nil {
		log.Errorln("Unable to get list of movies")
		return err
	}

	// get movies file names
	var fileNames []string
	for _, m := range moviesInDb {
		fileNames = append(fileNames, m.FileName)
	}

	// get list of removed files
	var removedFiles []string
	for _, f := range fileNames {
		if !s.sliceContains(videos, f) {
			if err := s.repo.RemoveMovieByFileName(f); err != nil {
				log.Errorf("Unable to remove file [file_name=%s]: %v", f, err)
				continue
			}
			removedFiles = append(removedFiles, f)
		}
	}
	if len(removedFiles) > 0 {
		log.Warnf("Removed files since last update: [%s]", strings.Join(removedFiles, ", "))
	}

	for _, v := range videos {
		wg.Add(1)
		go func(video string) {
			defer wg.Done()

			if err := s.updateMovie(video); err != nil {
				log.Errorf("Unable to update movie [file_name=%s]: %v", video, err)
			}
		}(v)
	}

	wg.Wait()
	log.Infoln("The movie database has been updated.")
	return nil
}

func (s *videoService) SaveTVShows() error {
	log.Infoln("Updating series database... ")
	// check if video dir path ends with slash
	videoDirPath := env.EnvString("VIDEO_DIR")
	if !strings.HasSuffix(videoDirPath, "/") {
		videoDirPath += "/"
	}

	tvSeriesList, err := s.getTvSeries(videoDirPath)
	if err != nil {
		log.Errorln("Unable to get tv series list")
		return err
	}

	var wg sync.WaitGroup

	for _, t := range tvSeriesList {
		wg.Add(1)
		go func(tvSeriesDir string) {
			defer wg.Done()

			if err := s.updateTvSeries(tvSeriesDir); err != nil {
				log.Errorf("Unable to update TV series [tv_series_dir=%s]: %v", tvSeriesDir, err)
			}
		}(t)
	}

	wg.Wait()

	log.Infoln("TV series database has been updated.")
	return nil
}

func (s *videoService) AllMovies() ([]*Movie, error) {
	movies, err := s.repo.FindAllMovies()
	if err != nil {
		log.Errorf("Unable to get all movies: %v", err)
		return nil, err
	}

	log.Infoln("Successfully found all movies")
	return movies, nil
}

func (s *videoService) AllTvSeries() ([]*TVSeries, error) {
	tvSeries, err := s.repo.FindAllTvSeries()
	if err != nil {
		log.Errorf("Unable to get all tv series: %v", err)
		return nil, err
	}
	log.Infoln("Successfully found all TV series")
	return tvSeries, nil
}

func (s *videoService) TvSeriesEpisodes(title string) ([]*Season, error) {
	videoDirPath := env.EnvString("VIDEO_DIR")
	if !strings.HasSuffix(videoDirPath, "/") {
		videoDirPath += "/"
	}

	/* Get seasons */
	var seasonsNames []string
	tvSeriesDir := videoDirPath + title + "/"
	files, err := ioutil.ReadDir(tvSeriesDir)
	if err != nil {
		log.Errorf("Error while scanning seasons for tv series [tv_series_dir=%s]: %v", tvSeriesDir, err)
		return nil, err
	}
	for _, f := range files {
		if f.IsDir() {
			seasonsNames = append(seasonsNames, f.Name())
		}
	}

	/* Get season episodes */
	var seasons []*Season
	for _, s := range seasonsNames {
		files, err := ioutil.ReadDir(tvSeriesDir + s)
		if err != nil {
			log.Errorf("Error while scanning for episodes [episodes_dir=%s]: %v", tvSeriesDir+s, err)
			return nil, err
		}
		// get episodes
		var episodes []string
		for _, f := range files {
			if strings.HasSuffix(f.Name(), "mp4") || strings.HasSuffix(f.Name(), "mkv") {
				episodes = append(episodes, f.Name())
			}
		}
		// add season to list
		s := Season{
			Name:     s,
			Episodes: episodes,
		}
		seasons = append(seasons, &s)
	}

	log.Infof("Successfully found TV series episodes [tv_series_title=%s]", title)
	return seasons, nil
}

func (s *videoService) getVideos(videoDirPath string) ([]string, error) {

	/* Get movies from disk (mkv & mp4 files).*/
	var videos []string
	files, err := ioutil.ReadDir(videoDirPath)
	if err != nil {
		log.Errorf("Error while scanning for videos [video_dir=%s]: %v", videoDirPath, err)
		return nil, err
	}
	for _, f := range files {
		if strings.HasSuffix(f.Name(), "mkv") || strings.HasSuffix(f.Name(), "mp4") {
			videos = append(videos, f.Name())
		}
	}

	return videos, nil
}

func (s *videoService) getTvSeries(tvSeriesDirPath string) ([]string, error) {
	var tvSeries []string
	files, err := ioutil.ReadDir(tvSeriesDirPath)
	if err != nil {
		log.Errorf("Error while scanning for tv series [tv_series_dir=%s]: %v", tvSeriesDirPath, err)
		return nil, err
	}
	for _, f := range files {
		if f.IsDir() && f.Name() != "sub" && f.Name() != "scripts" {
			tvSeries = append(tvSeries, f.Name())
		}
	}

	return tvSeries, nil
}

func (s *videoService) getMovieAndTvSeriesInfo(fileName string) (*Movie, *TVSeries, error) {
	toRemove := []string{".NSB", ".mkv", ".mp4"}
	var toSearch = s.removeFromArray(fileName, toRemove)

	/* Get movie info from filmweb.pl TODO: allow user to choose other service*/
	var url string

	// if file is probably tv series
	if !strings.Contains(fileName, "mp4") {
		url = "https://filmweb.pl/serials/search?q=" + toSearch
	} else {
		url = "https://filmweb.pl/search?q=" + toSearch
	}

	res, err := soup.Get(url)
	if err != nil {
		log.Errorf("Unable to get [url=%s]: %v", url, err)
		return nil, nil, err
	}

	doc := soup.HTMLParse(res)

	/* Get movie card and check for errors. */
	movieCard := doc.Find("ul", "class", "hits")
	if movieCard.Error != nil {
		log.WithFields(log.Fields{"movie": toSearch}).Error("Cannot find results list")
		return nil, nil, err
	}
	movieCard = movieCard.Find("li")
	if movieCard.Error != nil {
		log.WithFields(log.Fields{"movie": toSearch}).Error("Cannot find movie")
		return nil, nil, err
	}

	/* Get movie title */
	titleHTML := movieCard.Find("data")
	if titleHTML.Error != nil {
		log.WithFields(log.Fields{"movie": toSearch}).Error("Cannot find movie title")
		return nil, nil, err
	}
	title := titleHTML.Attrs()["data-title"]

	/* Get movie release date */
	movieReleaseDateHTML := movieCard.Find("div")
	if movieReleaseDateHTML.Error != nil {
		log.WithFields(log.Fields{"movie": toSearch}).Error("Cannot find movie release date")
		return nil, nil, err
	}
	movieReleaseDate := movieReleaseDateHTML.Attrs()["data-release"]

	/* Get movie duration */
	movieDurationHTML := movieCard.Find("div", "class", "filmPreview__filmTime")
	if movieDurationHTML.Error != nil {
		log.WithFields(log.Fields{"movie": toSearch}).Error("Cannot find movie duration")
		return nil, nil, err
	}
	movieDuration := movieDurationHTML.Text()

	/* Get movie rate */
	movieRateHTML := movieCard.Find("div", "class", "filmPreview__rateBox")
	if movieRateHTML.Error != nil {
		log.WithFields(log.Fields{"movie": toSearch}).Error("Cannot find movie rate")
		return nil, nil, err
	}
	movieRate := movieRateHTML.Attrs()["data-rate"]
	// convert movie rate to float
	movieRateFloat, err := strconv.ParseFloat(movieRate, 64)
	if err != nil {
		return nil, nil, err
	}

	/* Get movie director */
	movieDirectorHTML := movieCard.Find("div", "class", "filmPreview__info--directors")
	if movieDurationHTML.Error != nil {
		log.WithFields(log.Fields{"movie": toSearch}).Error("Cannot find movie directors")
		return nil, nil, err
	}
	movieDirectorHTML = movieDirectorHTML.Find("a")
	if movieDurationHTML.Error != nil {
		log.WithFields(log.Fields{"movie": toSearch}).Error("Cannot find movie directors")
		return nil, nil, err
	}
	movieDirector := movieDirectorHTML.Attrs()["title"]

	/* Get movie genre */
	movieGenreHTML := movieCard.Find("div", "class", "filmPreview__info--genres")
	if movieGenreHTML.Error != nil {
		log.WithFields(log.Fields{"movie": toSearch}).Error("Cannot find movie genre")
		return nil, nil, err
	}
	movieGenreHTML = movieGenreHTML.Find("ul")
	if movieGenreHTML.Error != nil {
		log.WithFields(log.Fields{"movie": toSearch}).Error("Cannot find movie genre")
		return nil, nil, err
	}
	movieGenreHTML = movieGenreHTML.Find("a")
	if movieGenreHTML.Error != nil {
		log.WithFields(log.Fields{"movie": toSearch}).Error("Cannot find movie genre")
		return nil, nil, err
	}
	movieGenre := movieGenreHTML.Text()

	/* Get movie poster */
	moviePosterHTML := movieCard.Find("img", "class", "filmPoster__image")
	if moviePosterHTML.Error != nil {
		log.WithFields(log.Fields{"movie": toSearch}).Error("Cannot find movie poster")
		return nil, nil, err
	}
	moviePoster := moviePosterHTML.Attrs()["data-src"]
	moviePoster = strings.Replace(moviePoster, "6.jpg", "3.jpg", -1)

	/* Get movie details (description) */
	detailsLinkHTML := movieCard.Find("a", "class", "filmPreview__link")
	if detailsLinkHTML.Error != nil {
		log.WithFields(log.Fields{"movie": toSearch}).Error("Cannot find details link")
		return nil, nil, err
	}
	// Scrape details page
	detailsURL := detailsLinkHTML.Attrs()["href"]
	detailsRes, err := soup.Get("https://filmweb.pl" + detailsURL)
	if err != nil {
		log.WithFields(log.Fields{"movie": toSearch}).Error("Unable to open details page")
		return nil, nil, err
	}
	detailsDoc := soup.HTMLParse(detailsRes)
	// Get movie description
	descriptionHTML := detailsDoc.Find("div", "class", "filmPlot")
	if descriptionHTML.Error != nil {
		log.WithFields(log.Fields{"movie": toSearch}).Error("Cannot find movie description")
		return nil, nil, err
	}
	descriptionHTML = descriptionHTML.Find("p")
	if descriptionHTML.Error != nil {
		log.WithFields(log.Fields{"movie": toSearch}).Error("Cannot find movie description")
		return nil, nil, err
	}
	description := descriptionHTML.Text()

	// Get movie cast
	castURL := detailsURL + "/cast/actors"
	castRes, err := soup.Get("https://filmweb.pl" + castURL)
	if err != nil {
		log.WithFields(log.Fields{"movie": toSearch}).Error("Unable to open cast page")
		return nil, nil, err
	}

	castDoc := soup.HTMLParse(castRes)

	var cast []*Role
	// Get cast table
	castTable := castDoc.Find("table", "class", "filmCast")
	if castTable.Error != nil {
		log.WithFields(log.Fields{"movie": toSearch}).Error("Unable to find cast table")
		//	return nil, nil, err
	} else {

		castTable = castTable.Find("tbody")
		if castTable.Error != nil {
			log.WithFields(log.Fields{"movie": toSearch}).Error("Unable to find cast table")
			return nil, nil, err
		}
		rolesHTML := castTable.FindAll("tr")
		for _, roleHTML := range rolesHTML {
			if roleHTML.Error != nil {
				log.WithFields(log.Fields{"movie": toSearch}).Error("Unable to find table rows for a cast")
				return nil, nil, err
			}
			castProperties := roleHTML.FindAll("a")
			actorPictureHTML := castProperties[0]
			if actorPictureHTML.Error != nil {
				log.WithFields(log.Fields{"movie": toSearch}).Error("Unable to find actor picture")
				return nil, nil, err
			}
			actorPictureHTML = actorPictureHTML.Find("img")
			if actorPictureHTML.Error != nil {
				log.WithFields(log.Fields{"movie": toSearch}).Error("Unable to find actor picture")
				return nil, nil, err
			}
			actorName := castProperties[0].Attrs()["title"]
			// Get picture and real actor name
			actorPictureURL := actorPictureHTML.Attrs()["src"]
			if strings.HasSuffix(actorPictureURL, "plug.svg") {
				actorPictureURL = "-"
			} else {
				actorPictureArr := strings.Split(actorPictureURL, ".")
				actorPictureArr[3] = "1"
				actorPictureURL = strings.Join(actorPictureArr, ".")
			}

			characterHTML := roleHTML.Find("span")
			if characterHTML.Error != nil {
				log.WithFields(log.Fields{"movie": toSearch}).Error("Unable to find character")
				return nil, nil, err
			}
			var character string
			character = characterHTML.Text()

			role := &Role{
				ActorName:       actorName,
				ActorPictureURL: actorPictureURL,
				Character:       character,
			}

			cast = append(cast, role)
		}
	}

	movie := Movie{
		Title:       title,
		Description: description,
		Director:    movieDirector,
		Genre:       movieGenre,
		Duration:    movieDuration,
		Rate:        movieRateFloat,
		ReleaseDate: movieReleaseDate,
		FileName:    fileName,
		PosterPath:  moviePoster,
		Cast:        cast,
	}

	tvSeries := TVSeries{
		Title:           title,
		Description:     description,
		Director:        movieDirector,
		Genre:           movieGenre,
		EpisodeDuration: movieDuration,
		Rate:            movieRateFloat,
		ReleaseDate:     movieReleaseDate,
		DirName:         fileName,
		PosterPath:      moviePoster,
		Cast:            cast,
	}
	return &movie, &tvSeries, nil
}

func (s *videoService) MoviePath(title string) string {
	videoDirPath := env.EnvString("VIDEO_DIR")
	if !strings.HasSuffix(videoDirPath, "/") {
		videoDirPath += "/"
	}

	return videoDirPath + title

}

func (s *videoService) MovieSubtitles(title string) (string, error) {
	subDirPath := env.EnvString("MOVIES_SUB_DIR")
	if !strings.HasSuffix(subDirPath, "/") {
		subDirPath += "/"
	}
	var subFileName string
	if strings.Contains(title, ".mkv") {
		subFileName = strings.Replace(title, ".mkv", ".vtt", -1)
	} else {
		subFileName = strings.Replace(title, ".mp4", ".vtt", -1)
	}

	files, err := ioutil.ReadDir(subDirPath)
	if err != nil {
		log.Errorf("Error while scanning for subtitles [sub_dir=%s]: %v", subDirPath, err)
		return "", err
	}
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".vtt") && f.Name() == subFileName {
			log.Infof("Found subtitles for movie [title=%s]", title)
			return subDirPath + subFileName, nil
		}
	}
	log.Errorf("Cannot find subtitles file for movie %s", title)
	return "", errors.New("Unable to find subtitles for movie")
}

func (s *videoService) removeFromArray(str string, toRemove []string) string {
	for _, x := range toRemove {
		if strings.Contains(str, x) {
			str = strings.Replace(str, x, "", -1)
		}
	}
	return str
}

func (s *videoService) sliceContains(slice []string, item string) bool {
	for _, i := range slice {
		if item == i {
			return true
		}
	}
	return false
}

func (s *videoService) GetMovie(id string) (*Movie, error) {
	movie, err := s.repo.GetMovieById(id)
	if err != nil {
		log.Errorf("Unable to get movie [id=%s]: %v", id, err)
		return nil, err
	}
	log.Infof("Successfully found movie [id=%s, title=%s]", id, movie.Title)
	return movie, nil
}

func (s *videoService) GetTvSeries(id string) (*TVSeries, error) {
	tvSeries, err := s.repo.GetTvSeriesById(id)
	if err != nil {
		log.Errorf("Unable to get TV series [id=%s]: %v", id, err)
		return nil, err
	}
	log.Infof("Successfully found TV series [id=%s, title=%s]", id, tvSeries.Title)
	return tvSeries, nil
}
