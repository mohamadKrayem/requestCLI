# Request-CLI

Request-CLI, inspired by tools like httpie and curl, is designed to be beginner-friendly with strong JSON operations, making it easy to execute various HTTP requests. The app offers advanced features such as handling cookies, basic authentication, and the ability to format and colorize outputs.

# Installation 
Make sure to have go on your system from https://go.dev/doc/install. Check it is installed by running the command 
```
$ go version
```
You can install requestCLI by executing the command :
```
$ go install github.com/mohamadkrayem/requestCLI
```
There will be a docker container image soon......


# Usage

Hello World:
```
$ requestCLI get helloworld.com
```
See also 
```
$ requestCLI --help.
```

## Examples

Custom HTTP method, simple HTTP headers, simple JSON data as the body of the request:
```
$ requestCLI post example.com -n X-API-Token=123 -b='{"name":"Mohamad"}'
```
If you want to send multiple simple headers:
```
$ requestCLI post example.com -n X-API-Token-1=123 -n X-API-Token-2=456
```
or simply by separating the headers by a ' , ' :
```
$ requestCLI post example.com -n X-API-Token-1=123,X-API-Token-2=456
```
---
Custom HTTP method, complex HTTP headers use the '- -headers' flag, complex JSON data use the '- -body' flag:
```
$ requestCLI put example.com --headers --body
 {
	 "X-API-Token": 123
 };
 {
	 "name":"Mohamad",
	 "arrayOfNbs": [
		 1,
		 2,
		 3
	 ],
	 "nestedJS": {
		 "w":"2"
	 }
 };
  
```
---
Custom HTTP method, simple HTTP headers, with Cookies:
```
$ requestCLI get example.com -n X-API-Token=123 -c key=value 
```
---
Custom HTTP GET method, with Basic authentication:
```
$ requestCLI get example.com --auth username=password
```
---
Custom HTTP GET method, Querystring parameters:
```
$ requestCLI get example.com -q q=queryExample,per_page=1
```
---
Custom HTTP method, Output option:
```
$ requestCLI get example.com -H
```
- -H or --printH for headers
- -B or --printB for body
- -S or --printS for status
---
Delete:
```
$ requestCLI delete example.com
```
Post:
```
$ requestCLI post "http://example.com" -b='{"name":"Example"}'
```
Put:
```
$ requestCLI put example.com -b='{"name":"Example"}'
```
---
## Sending forms and files
Custom http request including form data with files:
```
$ requestCLI post example.com/form --multi --body
{  
	"@!image":"~/justForTesting/OIG.jpeg",  
	"@!resume":"~/justForTesting/Mohamad_Krayem_2023_CV.docx",  
	"@!letter":"~/justForTesting/letter.txt",  
	"name":"Mohamad",  
	"age":22  
};
```

All your files must begin with ' @! ' and the normal data fields without any symbole.
If your path starts with:
- ' ~ ': the app will automaticaly search in the home directory.
- ' / ': the app won't do any automatical behavior and it will go directly to the path.
- string : the app will search in the current working directory.
--- 
To send normal form:
```
$ requestCLI post example.com -f --body
{
	"key1":"value",
	"key2":21
};
```
>**Note**: You can use the -b or the --body flag to include a body in your request.
---

## Technologies Used

- Golang
- Cobra

This allowed for efficient CLI development and improved the user experience. I also gained valuable experience in backend development through this project.
---
## Future Features

The app is designed to handle future features such as:

- JWT authentication
- Digest authentication
- Proxies
- SSL certificates
- The ability to send and download files

These features will be added in future updates.

Overall, this app is designed to be a versatile and powerful tool for developers of all skill levels, making it easy to work with HTTP requests and handle a variety of common authentication and authorization scenarios.
---
## Thank You.