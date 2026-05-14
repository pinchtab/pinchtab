#!/usr/bin/env perl
use strict;
use warnings;
use IO::Socket::INET;

my $root = $ENV{FIXTURES_ROOT} || '/fixtures';
my $port = $ENV{FIXTURES_PORT} || 8080;

my $server = IO::Socket::INET->new(
    LocalAddr => '0.0.0.0',
    LocalPort => $port,
    Listen    => 20,
    ReuseAddr => 1,
) or die "listen on $port: $!";

$| = 1;
print "fixture server listening on $port\n";

while (my $client = $server->accept) {
    $client->autoflush(1);
    my $request = <$client> || '';
    while (my $header = <$client>) {
        last if $header =~ /^\r?\n$/;
    }

    if ($request !~ m{^GET\s+([^ ]+)\s+HTTP/}i) {
        respond($client, 405, 'text/plain; charset=utf-8', 'method not allowed');
        next;
    }

    my $path = $1;
    $path =~ s/\?.*\z//;
    $path =~ s/%([0-9A-Fa-f]{2})/chr(hex($1))/eg;
    $path = '/index.html' if $path eq '/';

    if ($path eq '/redirect-to-internal') {
        redirect($client, 'http://127.0.0.1:9999/health');
        next;
    }

    if ($path =~ /\0/ || $path =~ m{(^|/)\.\.(/|\z)}) {
        respond($client, 403, 'text/plain; charset=utf-8', 'forbidden');
        next;
    }

    $path =~ s{^/+}{};
    my $file = "$root/$path";
    if (!-f $file) {
        respond($client, 404, 'text/plain; charset=utf-8', 'not found');
        next;
    }

    my $fh;
    if (!open $fh, '<:raw', $file) {
        respond($client, 500, 'text/plain; charset=utf-8', 'open failed');
        next;
    }
    local $/;
    my $body = <$fh>;
    close $fh;

    respond($client, 200, content_type($file), $body);
}

sub content_type {
    my ($file) = @_;
    return 'text/html; charset=utf-8' if $file =~ /\.html\z/;
    return 'text/plain; charset=utf-8' if $file =~ /\.txt\z/;
    return 'application/javascript; charset=utf-8' if $file =~ /\.js\z/;
    return 'text/css; charset=utf-8' if $file =~ /\.css\z/;
    return 'application/xml' if $file =~ /\.xml\z/;
    return 'application/gzip' if $file =~ /\.gz\z/;
    return 'application/octet-stream';
}

sub respond {
    my ($client, $status, $content_type, $body) = @_;
    my %reason = (
        200 => 'OK',
        302 => 'Found',
        403 => 'Forbidden',
        404 => 'Not Found',
        405 => 'Method Not Allowed',
        500 => 'Internal Server Error',
    );
    $body = '' if !defined $body;
    print {$client} "HTTP/1.1 $status " . ($reason{$status} || 'OK') . "\r\n";
    print {$client} "Content-Type: $content_type\r\n";
    print {$client} "Content-Length: " . length($body) . "\r\n";
    print {$client} "Connection: close\r\n\r\n";
    print {$client} $body;
    close $client;
}

sub redirect {
    my ($client, $location) = @_;
    print {$client} "HTTP/1.1 302 Found\r\n";
    print {$client} "Location: $location\r\n";
    print {$client} "Content-Length: 0\r\n";
    print {$client} "Connection: close\r\n\r\n";
    close $client;
}
