-- +goose Up
CREATE TABLE users (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  email VARCHAR(255) NOT NULL UNIQUE,
  nickname VARCHAR(100) NOT NULL,
  password_hash VARCHAR(255) NOT NULL,
  role ENUM('reader','author','admin') NOT NULL DEFAULT 'reader',
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
);
CREATE TABLE sessions (
  token_hash CHAR(64) PRIMARY KEY,
  user_id BIGINT NOT NULL,
  expires_at DATETIME(6) NOT NULL,
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  INDEX idx_sessions_user (user_id)
);
CREATE TABLE articles (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  author_id BIGINT NOT NULL,
  title VARCHAR(255) NOT NULL,
  summary TEXT NOT NULL,
  body_md LONGTEXT NOT NULL,
  body_plain LONGTEXT NOT NULL,
  status ENUM('draft','published','unpublished','deleted') NOT NULL DEFAULT 'draft',
  previous_status ENUM('draft','published','unpublished') NULL,
  published_at DATETIME(6) NULL,
  deleted_at DATETIME(6) NULL,
  version BIGINT NOT NULL DEFAULT 1,
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  FOREIGN KEY (author_id) REFERENCES users(id),
  INDEX idx_articles_public (status, published_at, id),
  INDEX idx_articles_author (author_id, status, updated_at),
  FULLTEXT INDEX idx_articles_search (title, summary, body_plain) WITH PARSER ngram
);
CREATE TABLE tags (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  name VARCHAR(80) NOT NULL UNIQUE
);
CREATE TABLE article_tags (
  article_id BIGINT NOT NULL,
  tag_id BIGINT NOT NULL,
  PRIMARY KEY (article_id, tag_id),
  FOREIGN KEY (article_id) REFERENCES articles(id),
  FOREIGN KEY (tag_id) REFERENCES tags(id) ON DELETE CASCADE,
  INDEX idx_article_tags_tag (tag_id, article_id)
);
CREATE TABLE favorites (
  user_id BIGINT NOT NULL,
  article_id BIGINT NOT NULL,
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  PRIMARY KEY (user_id, article_id),
  FOREIGN KEY (user_id) REFERENCES users(id),
  FOREIGN KEY (article_id) REFERENCES articles(id)
);
CREATE TABLE comments (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  article_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  content VARCHAR(4000) NOT NULL,
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  FOREIGN KEY (article_id) REFERENCES articles(id),
  FOREIGN KEY (user_id) REFERENCES users(id),
  INDEX idx_comments_article (article_id, id)
);
CREATE TABLE images (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  uploader_id BIGINT NOT NULL,
  article_id BIGINT NULL,
  filename VARCHAR(100) NOT NULL UNIQUE,
  mime VARCHAR(30) NOT NULL,
  size_bytes BIGINT NOT NULL,
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  FOREIGN KEY (uploader_id) REFERENCES users(id),
  FOREIGN KEY (article_id) REFERENCES articles(id),
  INDEX idx_images_article (article_id)
);

-- +goose Down
DROP TABLE images;
DROP TABLE comments;
DROP TABLE favorites;
DROP TABLE article_tags;
DROP TABLE tags;
DROP TABLE articles;
DROP TABLE sessions;
DROP TABLE users;
