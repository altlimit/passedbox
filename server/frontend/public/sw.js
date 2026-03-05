// PassedBox Push Notification Service Worker

self.addEventListener('push', function (event) {
    var data = { title: 'PassedBox Keep-Alive', body: 'Time to check in!', url: '/' };

    try {
        if (event.data) {
            data = event.data.json();
        }
    } catch (e) {
        // Use defaults if payload parsing fails
    }

    var options = {
        body: data.body || 'Check-in reminder from PassedBox',
        icon: data.icon || '/favicon.ico',
        badge: data.badge || '/favicon.ico',
        tag: data.tag || 'passedbox-checkin',
        requireInteraction: true,
        data: { url: data.url || '/' },
        actions: [
            { action: 'checkin', title: 'Check In Now' },
            { action: 'dismiss', title: 'Dismiss' }
        ]
    };

    event.waitUntil(
        self.registration.showNotification(data.title || 'PassedBox Keep-Alive', options)
    );
});

self.addEventListener('notificationclick', function (event) {
    event.notification.close();

    var url = event.notification.data && event.notification.data.url ? event.notification.data.url : '/';

    // Handle "Check In Now" action
    if (event.action === 'checkin') {
        url = event.notification.data && event.notification.data.url ? event.notification.data.url : '/';
    }

    event.waitUntil(
        clients.matchAll({ type: 'window', includeUncontrolled: true }).then(function (clientList) {
            // If there's already a window open, focus it
            for (var i = 0; i < clientList.length; i++) {
                if ('focus' in clientList[i]) {
                    return clientList[i].focus();
                }
            }
            // Otherwise open a new window
            if (clients.openWindow) {
                return clients.openWindow(url);
            }
        })
    );
});

self.addEventListener('install', function (event) {
    self.skipWaiting();
});

self.addEventListener('activate', function (event) {
    event.waitUntil(self.clients.claim());
});
