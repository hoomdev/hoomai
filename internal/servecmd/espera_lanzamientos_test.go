package servecmd

// waitLaunches blocks until every role this Studio launched has finished.
// Only the tests wait for them: the server itself never does.
func (s *Server) waitLaunches() { s.launches.Wait() }
