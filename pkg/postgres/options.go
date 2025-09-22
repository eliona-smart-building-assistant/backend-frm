package postgres

type Opt func(*Pool)

func WithHostname(hostname string) Opt {
	return func(p *Pool) {
		p.hostname = hostname
	}
}

func WithPort(port int) Opt {
	return func(p *Pool) {
		p.port = port
	}
}

func AllowCredentialChange() Opt {
	return func(p *Pool) {
		p.allowCredentialChange = true
	}
}

func WithCredentials(login, password string) Opt {
	return func(p *Pool) {
		p.login = login
		p.password = password
	}
}

func WithDatabase(db string) Opt {
	return func(p *Pool) {
		p.database = db
	}
}

func WithDSN(dsn string) Opt {
	return func(p *Pool) {
		p.dsn = dsn
	}
}

func WithMaxPoolSize(max int) Opt {
	return func(p *Pool) {
		p.maxPoolSize = max
	}
}

func WithApplicationName(name string) Opt {
	return func(p *Pool) {
		p.appName = name
	}
}

func WithAsyncCommits() Opt {
	return func(p *Pool) {
		p.asyncCommits = true
	}
}

func WithOverrideRole(role string) Opt {
	return func(p *Pool) {
		p.overrideRole = role
	}
}
