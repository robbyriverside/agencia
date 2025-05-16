package agencia

type DialogPair struct {
	Input  string `yaml:"input"`
	Output string `yaml:"output"`
}

type Script struct {
	Dialog []DialogPair `yaml:"dialog"`
}

type Root struct {
	Scripts []Script `yaml:"scripts"`
}
