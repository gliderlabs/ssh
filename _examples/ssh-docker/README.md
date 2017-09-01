# SSH Docker Example
Run docker containers over SSH.  You can even pipe things into them too!

# Installation / Prep
We're going to build JQ as an SSH service using the Glider Labs SSH package.  If you haven't installed GoLang and docker yet, see the doc's for help getting your environment setup.

Install the Glider Labs SSH package  
`go get github.com/gliderlabs/ssh`

Build the example docker container with  
`docker build --rm -t jq .` 

# Usage
Run the SSH server with

`go run docker.go`  

This SSH service uses the user name passed into the connection to key the desired docker image.  So to use jq over our SSH server we would say:

`ssh jq@localhost -p 2222`  

If we run this command we will see

```
jq - commandline JSON processor [version 1.5]
Usage: jq [options] <jq filter> [file...]

	jq is a tool for processing JSON inputs, applying the
	given filter to its JSON text inputs and producing the
	filter's results as JSON on standard output.
	The simplest filter is ., which is the identity filter,
	copying jq's input to its output unmodified (except for
	formatting).
	For more advanced filters see the jq(1) manpage ("man jq")
	and/or https://stedolan.github.io/jq

	Some of the options include:
	 -c		compact instead of pretty-printed output;
	 -n		use `null` as the single input value;
	 -e		set the exit status code based on the output;
	 -s		read (slurp) all inputs into an array; apply filter to it;
	 -r		output raw strings, not JSON texts;
	 -R		read raw strings, not JSON texts;
	 -C		colorize JSON;
	 -M		monochrome (don't colorize JSON);
	 -S		sort keys of objects on output;
	 --tab	use tabs for indentation;
	 --arg a v	set variable $a to value <v>;
	 --argjson a v	set variable $a to JSON value <v>;
	 --slurpfile a f	set variable $a to an array of JSON texts read from <f>;
	See the manpage for more options.
Connection to localhost closed.
```

JQ's help text!  It's working!  Now let's pipe some json into our SSH service and marvel at the awesomeness.

`curl -s https://api.github.com/orgs/gliderlabs | ssh jq@localhost -p 2222 .`

```json
{
  "login": "gliderlabs",
  "id": 8484931,
  "url": "https://api.github.com/orgs/gliderlabs",
  "repos_url": "https://api.github.com/orgs/gliderlabs/repos",
  "events_url": "https://api.github.com/orgs/gliderlabs/events",
  "hooks_url": "https://api.github.com/orgs/gliderlabs/hooks",
  "issues_url": "https://api.github.com/orgs/gliderlabs/issues",
  "members_url": "https://api.github.com/orgs/gliderlabs/members{/member}",
  "public_members_url": "https://api.github.com/orgs/gliderlabs/public_members{/member}",
  "avatar_url": "https://avatars3.githubusercontent.com/u/8484931?v=4",
  "description": "",
  "name": "Glider Labs",
  "company": null,
  "blog": "http://gliderlabs.com",
  "location": "Austin, TX",
  "email": "team@gliderlabs.com",
  "has_organization_projects": true,
  "has_repository_projects": true,
  "public_repos": 29,
  "public_gists": 0,
  "followers": 0,
  "following": 0,
  "html_url": "https://github.com/gliderlabs",
  "created_at": "2014-08-18T23:25:37Z",
  "updated_at": "2017-08-21T09:52:17Z",
  "type": "Organization"
}
```

# Conclusion
We built JQ as a service over SSH in Go using the Glider Labs SSH package.  We showed how you can run docker containers through the service as well as how to pipe stuff into your SSH service.  


# Troubleshooting

A new host key is generated each time we run the server so you'll probably see this error on the second run.  
```
@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@
@    WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED!     @
@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@
IT IS POSSIBLE THAT SOMEONE IS DOING SOMETHING NASTY!
Someone could be eavesdropping on you right now (man-in-the-middle attack)!
It is also possible that a host key has just been changed.
The fingerprint for the RSA key sent by the remote host is
SHA256:9dyZ8KrMCPGvDqECggoDFyz51gA6DdHa9zpfl/5Kgos.
Please contact your system administrator.
Add correct host key in /Users/murr/.ssh/known_hosts to get rid of this message.
Offending RSA key in /Users/murr/.ssh/known_hosts:7
RSA host key for [localhost]:2222 has changed and you have requested strict checking.
Host key verification failed.
```

To bypass this error, regnerate your host key with  
`ssh-keygen -R "[localhost]:2222"`
