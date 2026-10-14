#!/bin/sh
python -c "
try:
    path = '/code/plane/app/views/base.py'
    with open(path, 'r') as f:
        c = f.read()
    if 'APIKeyAuthentication' not in c:
        c = c.replace('from plane.authentication.session import BaseSessionAuthentication', 'from plane.authentication.session import BaseSessionAuthentication\nfrom plane.api.middleware.api_authentication import APIKeyAuthentication')
        c = c.replace('authentication_classes = [\n        BaseSessionAuthentication,\n    ]', 'authentication_classes = [\n        BaseSessionAuthentication,\n        APIKeyAuthentication,\n    ]')
        with open(path, 'w') as f:
            f.write(c)
        print('[Plane] Applied APIKeyAuthentication patch to BaseViewSet')
except Exception as e:
    print('[Plane] BaseViewSet Patch error:', e)

try:
    rl_path = '/code/plane/api/rate_limit.py'
    with open(rl_path, 'r') as f:
        rc = f.read()
    if '60/minute' in rc:
        rc = rc.replace('rate = \"60/minute\"', 'rate = \"6000/minute\"')
        with open(rl_path, 'w') as f:
            f.write(rc)
        print('[Plane] Relaxed ApiKeyRateThrottle limit to 6000/minute')
except Exception as e:
    print('[Plane] Rate limit patch error:', e)
"
exec python manage.py runserver 0.0.0.0:${PORT:-8090}
