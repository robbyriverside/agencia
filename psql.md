Here’s a compact and comprehensive PostgreSQL psql Cheat Sheet in Markdown format:

⸻



# 🐘 PostgreSQL `psql` Cheat Sheet

A quick reference for using the PostgreSQL CLI (`psql`).

## 📚 Installation

```bash
brew install postgresql
```

```bash
brew services start postgresql
```

```bash
createuser --interactive 
createdb agencia
```

---

## 🔐 Connect to PostgreSQL

```bash
psql -U postgres
psql -U your_user -h localhost -p 5432

🔄 General Session Commands

Command	Description
\q	Quit psql
\!	Run shell command
\c dbname	Connect to a database
\conninfo	Show current connection info
\password	Change your password



⸻

📂 Database & Role Management

Command	Description
\l / \list	List all databases
\du	List all roles (users)
CREATE DATABASE name;	Create a new database
CREATE ROLE name;	Create a new role
DROP DATABASE name;	Delete a database



⸻

📦 Schema & Tables

Command	Description
\dt	List tables in the current schema
\d tablename	Describe a table
\dn	List all schemas
SET search_path TO myschema;	Use schema
CREATE TABLE ...	Define new table



⸻

📊 Data Queries

SELECT * FROM tablename;
INSERT INTO tablename (...) VALUES (...);
UPDATE tablename SET ... WHERE ...;
DELETE FROM tablename WHERE ...;



⸻

🔍 Introspection & Debugging

Command	Description
\x	Expanded output mode (toggle)
\timing	Show execution time of commands
SELECT version();	PostgreSQL version
SELECT current_database();	Current database
SELECT * FROM pg_stat_activity;	Show active connections



⸻

🧠 Useful Tips
	•	Use ; to terminate SQL statements.
	•	Use \i filename.sql to run SQL file.
	•	Use \echo to print values inside scripts.

⸻

💡 Resources
	•	Official Docs: https://www.postgresql.org/docs/
	•	psql Help: Run \? inside the prompt

Would you like this saved to a file or enhanced with real examples?