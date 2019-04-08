package video

type VideoRepository interface {
	// SaveMovie saves movie to the database
	SaveMovie(movie *Movie) error
	// FindAllMovies returns list of all movies from the database
	FindAllMovies() ([]*Movie, error)
	// SaveTvSeries saves tv series to the database
	SaveTvSeries(tvSeries *TVSeries) error
}
