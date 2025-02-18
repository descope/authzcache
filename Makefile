ifneq ("$(wildcard vendor)","")
  include vendor/github.com/descope/common/scripts/dev/Makefile
else ifneq ("$(GOPATH)","")
  include $(GOPATH)/src/github.com/descope/common/scripts/dev/Makefile
else
  $(error The common Makefile could not be found)
endif

antlr: # Genereate the parser files
	cd internal/services/dsl/antlr/grammar && antlr4 -Dlanguage=Go -o ../generated  -package generated Authz.g4
