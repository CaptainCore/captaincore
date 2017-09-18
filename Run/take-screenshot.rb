#!/usr/bin/env ruby

##
##     Take Screenshots using Chrome via Selenium
##
##     Pass arguments from command line like this
##     ruby take-screenshot.rb anchor.host
##

require "selenium-webdriver"
require "highline/import" # Used for command line input

# Load command line arguments
@website = ARGV[0]

# configure the driver to run in headless mode
options = Selenium::WebDriver::Chrome::Options.new
options.add_argument('--headless')
options.add_argument('--hide-scrollbars')
driver = Selenium::WebDriver.for :chrome, options: options

# navigate to a really super awesome blog
driver.navigate.to "http://"+@website

# resize the window and take a screenshot
driver.manage.window.resize_to(1200, 800)
driver.save_screenshot "Tmp/screenshot-"+@website+".png"
