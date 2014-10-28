package Net::NTP;
#$Header: /home/cvs/Net-NTP/Net/NTP/NTP.pm,v 1.2 2004/02/23 17:53:47 jim Exp $

use 5.008;
use strict;
use warnings;
use Carp;

require Exporter;

our @ISA = qw(Exporter);

our @EXPORT = qw(
	get_ntp_response
);

#hack found using Google; MessageID <3C955A7D.D12D160C@earthlink.net>
#modified to give only a 2 digit version number
our $VERSION = sprintf "%d.%d", q$Revision: 1.2 $ =~ /(\d+)\.(\d+)/g;

our $CLIENT_TIME_SEND = undef;
our $CLIENT_TIME_RECEIVE = undef;

our $TIMEOUT = 60,

our %MODE = (
      '0'    =>    'reserved',
      '1'    =>    'symmetric active',
      '2'    =>    'symmetric passive',
      '3'    =>    'client',
      '4'    =>    'server',
      '5'    =>    'broadcast',
      '6'    =>    'reserved for NTP control message',
      '7'    =>    'reserved for private use'
);

our %STRATUM = (
      '0'          =>    'unspecified or unavailable',
      '1'          =>    'primary reference (e.g., radio clock)',
);

for(2 .. 15){
    $STRATUM{$_} = 'secondary reference (via NTP or SNTP)';
}

for(16 .. 255){
    $STRATUM{$_} = 'reserved';
}

our %STRATUM_ONE_TEXT = (
    'LOCL'    => 'uncalibrated local clock used as a primary reference for a subnet without external means of synchronization',
    'PPS'     => 'atomic clock or other pulse-per-second source individually calibrated to national standards',
    'ACTS'  => 'NIST dialup modem service',
    'USNO'  => 'USNO modem service',
    'PTB'   => 'PTB (Germany) modem service',
    'TDF'   => 'Allouis (France) Radio 164 kHz',
    'DCF'   => 'Mainflingen (Germany) Radio 77.5 kHz',
    'MSF'   => 'Rugby (UK) Radio 60 kHz',
    'WWV'   => 'Ft. Collins (US) Radio 2.5, 5, 10, 15, 20 MHz',
    'WWVB'  => 'Boulder (US) Radio 60 kHz',
    'WWVH'  => 'Kaui Hawaii (US) Radio 2.5, 5, 10, 15 MHz',
    'CHU'   => 'Ottawa (Canada) Radio 3330, 7335, 14670 kHz',
    'LORC'  => 'LORAN-C radionavigation system',
    'OMEG'  => 'OMEGA radionavigation system',
    'GPS'   => 'Global Positioning Service',
    'GOES'  => 'Geostationary Orbit Environment Satellite',
);

our %LEAP_INDICATOR = (
      '0'    =>     'no warning',
      '1'    =>     'last minute has 61 seconds',
      '2'    =>     'last minute has 59 seconds)',
      '3'    =>     'alarm condition (clock not synchronized)'
);

{

    use constant NTP_ADJ => 2208988800;

    my @ntp_packet_fields =
    (
        'Leap Indicator',
        'Version Number',
        'Mode',
        'Stratum',
        'Poll Interval',
        'Precision',
        'Root Delay',
        'Root Dispersion',
        'Reference Clock Identifier',
        'Reference Timestamp',
        'Originate Timestamp',
        'Receive Timestamp',
        'Transmit Timestamp',
    );

    my $frac2bin = sub {
        my $bin  = '';
        my $frac = shift;
        while ( length($bin) < 32 ) {
            $bin  = $bin . int( $frac * 2 );
            $frac = ( $frac * 2 ) - ( int( $frac * 2 ) );
        }
        return $bin;
    };

    my $bin2frac = sub {
        my @bin = split '', shift;
        my $frac = 0;
        while (@bin) {
            $frac = ( $frac + pop @bin ) / 2;
        }
        return $frac;
    };

    my $percision = sub{
        my $number = shift;
        if($number > 127){
            $number -= 255;
        }
        return sprintf("%1.4e", 2**$number);
    };

    my $unpack_ip = sub {
        my $ip;
        my $stratum = shift;
        my $tmp_ip = shift;
        if($stratum < 2){
            $ip = unpack("A4",
                pack("H8", $tmp_ip)
            );
        }else{
            $ip = sprintf("%d.%d.%d.%d",
                unpack("C4",
                    pack("H8", $tmp_ip)
                )
            );
        }
        return $ip;
    };

sub get_ntp_response{
    use IO::Socket;

    my $host = shift || 'localhost';
    my $port = shift || 'ntp';

    my $sock = IO::Socket::INET->new(
        Proto    => 'udp',
        PeerHost => $host,
        PeerPort => $port )
    or die $@;

    my %tmp_pkt;
    my %packet;
    my $data;


    $CLIENT_TIME_SEND = time() unless defined $CLIENT_TIME_SEND;
    my $client_localtime      = $CLIENT_TIME_SEND;
    my $client_adj_localtime  = $client_localtime + NTP_ADJ;
    my $client_frac_localtime = $frac2bin->($client_adj_localtime);

    my $ntp_msg =
      pack( "B8 C3 N10 B32", '00011011', (0) x 12, int($client_localtime),
      $client_frac_localtime );

    $sock->send($ntp_msg)
        or die "send() failed: $!\n";

    eval{
    local $SIG{ALRM} = sub { die "Net::NTP timed out geting NTP packet\n"; };
    alarm($TIMEOUT);
    $sock->recv($data,960)
        or die "recv() failed: $!\n";
    alarm(0)
    };

    if($@){
        die "$@";
    }

    $CLIENT_TIME_RECEIVE = time() unless defined $CLIENT_TIME_RECEIVE;

    my @ntp_fields = qw/byte1 stratum poll precision/;
    push @ntp_fields, qw/delay delay_fb disp disp_fb ident/;
    push @ntp_fields, qw/ref_time ref_time_fb/;
    push @ntp_fields, qw/org_time org_time_fb/;
    push @ntp_fields, qw/recv_time recv_time_fb/;
    push @ntp_fields, qw/trans_time trans_time_fb/;

    @tmp_pkt{@ntp_fields} =
        unpack( "a C3   n B16 n B16 H8   N B32 N B32   N B32 N B32", $data );

    @packet{@ntp_packet_fields} = (
        (unpack( "C", $tmp_pkt{byte1} & "\xC0" ) >> 6),
        (unpack( "C", $tmp_pkt{byte1} & "\x38" ) >> 3),
        (unpack( "C", $tmp_pkt{byte1} & "\x07" )),
        $tmp_pkt{stratum},
        (sprintf("%0.4f", $tmp_pkt{poll})),
        $tmp_pkt{precision} - 255,
        ($bin2frac->($tmp_pkt{delay_fb})),
        (sprintf("%0.4f", $tmp_pkt{disp})),
        $unpack_ip->($tmp_pkt{stratum}, $tmp_pkt{ident}),
        (($tmp_pkt{ref_time} += $bin2frac->($tmp_pkt{ref_time_fb})) -= NTP_ADJ),
        (($tmp_pkt{org_time} += $bin2frac->($tmp_pkt{org_time_fb})) ),
      (($tmp_pkt{recv_time} += $bin2frac->($tmp_pkt{recv_time_fb})) -= NTP_ADJ),
     (($tmp_pkt{trans_time} += $bin2frac->($tmp_pkt{trans_time_fb})) -= NTP_ADJ)
    );

    return %packet;
}

}

1;
__END__

=head1 NAME

Net::NTP - Perl extension for decoding NTP server responses

=head1 SYNOPSIS

  use Net::NTP;
  my %response = get_ntp_response();

=head1 ABSTRACT

All this module does is send a packet to an NTP server and then decode
the packet recieved into it's respective parts - as outlined in RFC1305
and RFC2030.

=head1 DESCRIPTION

This module exports a single method (get_ntp_response) and returns an associative array based upon RFC1305 and RFC2030.  The response from the server is "humanized" to a point that further processing of th information recieved from the server can be manipulated.  For example: timestamps are in epoch, so one could use the localtime function to produce an even more "human" representation of the timestamp.

=head2 EXPORT

get_ntp_resonse(<server>, <port>);

This module exports a single method - get_ntp_response.  It takes the server as the first argument (localhost is the default) and port to send/recieve the packets (ntp or 123 bu default).  It returns an associative array of the various parts of the packet as outlined in RFC1305.  It "normalizes" or "humanizes" various parts of the packet.  For example: all the timestamps are in epoch, NOT hexidecimal.

=head1 SEE ALSO

perl, IO::Socket, RFC1305, RFC2030

=head1 AUTHOR

James G. Willmore, E<lt>jwillmore (at) adelphia.net<gt> or E<lt>owner (at) ljcomputing.net<gt>

Special thanks to Ralf D. Kloth E<lt>ralf (at) qrq.de<gt> for the code to decode NTP packets.

=head1 COPYRIGHT AND LICENSE

Copyright 2004 by James G. Willmore

This library is free software; you can redistribute it and/or modify
it under the same terms as Perl itself.

=head1 CHANGE LOG
$Log: NTP.pm,v $
Revision 1.2  2004/02/23 17:53:47  jim
Modified regular expression used to produce version number.

Revision 1.1.1.1  2004/02/23 17:11:44  jim
Imported Net::NTP into CVS


=cut