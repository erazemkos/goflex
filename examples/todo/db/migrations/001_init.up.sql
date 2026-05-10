create table if not exists users (
  id integer primary key autoincrement,
  email text not null unique,
  name text,
  password_hash text not null,
  created_at datetime,
  updated_at datetime
);

create table if not exists todos (
  id integer primary key autoincrement,
  user_id integer not null,
  title text not null,
  done boolean not null default false,
  created_at datetime,
  updated_at datetime,
  unique(user_id, title),
  foreign key(user_id) references users(id) on delete cascade
);

create index if not exists idx_todos_user_id on todos(user_id);
