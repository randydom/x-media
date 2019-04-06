package mysql

import (
	"database/sql"

	"github.com/0x113/x-media/video"
	log "github.com/sirupsen/logrus"
)

type videoRepository struct {
	db     *sql.DB
	jwtKey string
}

func NewMySQLVideoRepository(db *sql.DB, jwtKey string) video.VideoRepository {
	return &videoRepository{
		db,
		jwtKey,
	}
}

func (r *videoRepository) SaveMovie(movie *video.Movie) error {
	query := "INSERT INTO movie (title, description, director, genre, duration, rate, release_date, poster_path) VALUES (?, ?, ?, ?, ?, ?, ?, ?) ON DUPLICATE KEY UPDATE title=?, description=?, director=?, genre=?, duration=?, rate=?, release_date=?, poster_path=?"
	stmt, err := r.db.Prepare(query)
	if err != nil {
		log.Errorf("Error while preparing statement: %s", err.Error())
		return err
	}

	res, err := stmt.Exec(movie.Title, movie.Description, movie.Director, movie.Genre, movie.Duration, movie.Rate, movie.ReleaseDate, movie.PosterPath, movie.Title, movie.Description, movie.Director, movie.Genre, movie.Duration, movie.Rate, movie.ReleaseDate, movie.PosterPath)
	if err != nil {
		log.Errorf("Error while executing statement: %s", err.Error())
		return err
	}
	newID, err := res.LastInsertId()
	if err != nil {
		return err
	}

	log.Infof("Created movie with id %d", newID)
	return nil

}

func (r *videoRepository) FindAllMovies() ([]*video.Movie, error) {
	rows, err := r.db.Query("SELECT * FROM movie")
	if err != nil {
		log.Errorf("Error while selecting all movies: %s", err.Error())
		return nil, err
	}
	defer rows.Close()

	var movies []*video.Movie
	for rows.Next() {
		movie := new(video.Movie)
		if err := rows.Scan(&movie.MovieID, &movie.Title, &movie.Description, &movie.Director, &movie.Genre, &movie.Duration, &movie.Rate, &movie.ReleaseDate, &movie.PosterPath); err != nil {
			log.Errorf("Error while scanning for movie: %s", err.Error())
			return nil, err
		}
		movies = append(movies, movie)
	}

	return movies, nil
}
