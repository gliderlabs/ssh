.PHONY: test example-deps circleci

NAME=ssh
OWNER=gliderlabs

test:
	go test -v -race

example-deps:
	go get -u github.com/docker/docker
	go get -u github.com/kr/pty

circleci:
ifdef CIRCLECI
	rm ~/.gitconfig
	rm -rf /home/ubuntu/.go_workspace/src/github.com/$(OWNER)/$(NAME) && cd .. \
		&& mkdir -p /home/ubuntu/.go_workspace/src/github.com/$(OWNER) \
		&& mv $(NAME) /home/ubuntu/.go_workspace/src/github.com/$(OWNER)/$(NAME) \
		&& ln -s /home/ubuntu/.go_workspace/src/github.com/$(OWNER)/$(NAME) $(NAME)
endif
