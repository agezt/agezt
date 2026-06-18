import urllib.request
r = urllib.request.urlopen('https://models.dev/api.json', timeout=10)
data = r.read().decode()
idx = data.find('"id":"302ai"')
if idx >= 0:
    print(data[idx-200:idx+800])
else:
    print("NOT FOUND")
