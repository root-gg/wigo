package Wigo::Probe;

use strict;
use warnings;

use Getopt::Long;
use JSON::XS;
use File::Basename;

require Exporter;
our @ISA = qw/Exporter/;
our @EXPORT_OK = qw/init config args result version status value message metrics add_metric detail raise persist output debug/;
our %EXPORT_TAGS = ( all => \@EXPORT_OK );

###
# VARS
###

my $CONFIG_PATH     = "/etc/wigo/conf.d";
my $PERSIST_PATH    = "/tmp";

my $version    = "0.10";

my  $name       = basename($0);
my  $config     = {};
my  $args       = [];
my  $persist    = undef;

my  $result     =  {
    Version     => "0.10",

    Status      => 100,
    Value       => undef,
    Message     => "",

    Detail      => {},
    Metrics     => [],
};

###
# COMMAND LINE OPTIONS
###

my $opts = {};
GetOptions (
    $opts,
    'debug',
    '<>' => sub { push @$args, $_[0] }
) or die("Error in command line arguments\n");

my $json = JSON::XS->new;
if ( exists $opts->{'debug'} )
{
    $json = JSON::XS->new->pretty;
}

###
# DEBUG
###

sub debug {
    if ( exists $opts->{'debug'} )
    {
        print shift;
    }
}

###
# OUTPUT JSON
###

sub output {
    my $code = shift;

    if ( defined $result->{'Value'} )
    {
        $result->{'Value'} .= "";
    }
    else
    {
        if ( $result->{"Status"} == 100 )
        {
            $result->{'Value'} = 'OK';
        }
        elsif ( $result->{"Status"} > 100 and $result->{"Status"} < 199 )
        {
            $result->{'Value'} = 'INFO';
        }
        elsif ( $result->{"Status"} >= 200 and $result->{"Status"} < 300 )
        {
            $result->{'Value'} = 'WARN';
        }
        elsif ( $result->{"Status"} >= 300 and $result->{"Status"} < 500 )
        {
            $result->{'Value'} = 'CRIT';
        }
        else
        {
            $result->{'Value'} = 'ERROR';
        }
    }

    for my $metric ( @{$result->{'Metrics'}} )
    {
        defined $metric->{'Value'} and $metric->{'Value'} += 0;
    }

    save();
    print $json->encode( $result ) . "\n";

    if ( defined $code )
    {
        exit $code;
    }
}

###
# GETTER / SETTERS
###

sub init {
    my %params = @_;

    load_config($params{'config'});
    restore();
}

sub config
{
    return $config;
}

sub args
{
    return $args;
}

sub result
{
    return $result;
}

sub version
{
    if ( $@ )
    {
        $result->{"Version"} = shift;
    }
    else
    {
        return $result->{"Version"};
    }
}

sub status
{
    if ( @_ )
    {
        $result->{"Status"} = shift;
    }
    else
    {
        return $result->{"Status"};
    }
}

sub value
{
    if ( @_ )
    {
        $result->{"Value"} = shift;
    }
    else
    {
        return $result->{"Value"};
    }
}

sub message
{
    if ( @_ )
    {
        $result->{"Message"} = shift;
    }
    else
    {
        return $result->{"Message"};
    }
}

sub metrics
{
    if ( @_ )
    {
        $result->{"Metrics"} = shift;
    }
    else
    {
        return $result->{"Metrics"};
    }
}

sub add_metric
{
    push @{$result->{"Metrics"}}, shift;
}

sub detail
{
    if ( @_ )
    {
        $result->{"Detail"} = shift;
    }
    else
    {
        return $result->{"Detail"};
    }
}

sub persist
{
    if ( @_ )
    {
        $persist = shift;
    }
    else
    {
        return $persist;
    }
}

sub raise {
    my $status  = shift;

    result->{'Status'} = $status if result->{'Status'} < $status;
}

###
# CONFIG
###

sub save_config
{
    my $json = JSON::XS->new->pretty;

    my $path = shift || $CONFIG_PATH . "/" . $name . ".conf";

    if ( open CONFIG, '>', $path )
        {
            eval {
                print CONFIG $json->encode($config)."\n";
            };
            close CONFIG;
    
            if ( $@ )
            {
                status 300;
                message "can't serialize config : $@";
                output 1;
            }
        }
        else
        {
            status 300;
            message "can't open config file $path for writing : $!";
            output 1;
        }
    
}

sub load_config
{
    my $path = $CONFIG_PATH . "/" . $name . ".conf";

    if ( -r $path )
    {
        if ( ! open JSON_CONFIG, '<', $path )
        {
            status  500;
            message "Error while opening json config file for read : " . $!;
            output  1;
        }

        my $json = join '', (<JSON_CONFIG>);
        close JSON_CONFIG;

        eval {
            $config = decode_json( $json );
        };

        if ( $@ )
        {
            status  500;
            message "Error while decoding json config: " . $@;
            output  1;
        }
    }
    else
    {
        $config = shift || {};
    }
}

###
# SAVE / LOAD PERSISTANT DATA
###

sub save
{
    return unless $persist;

    my $path = $PERSIST_PATH . "/" . $name . ".wigo";

    if ( open PERSIST, '>', $path )
    {
        eval {
            print PERSIST $json->encode($persist)."\n";
        };
        close PERSIST;

        if ( $@ )
        {
            status 300;
            message "can't serialize persistant data : $@";
            output 1;
        }
    }
    else
    {
        status 300;
        message "can't open persistant data file $path for writing : $!";
        output 1;
    }
}

sub restore
{
    my $path = $PERSIST_PATH . "/" . $name . ".wigo";

    return unless -e $path;

    if ( open PERSIST, '<', $path )
    {
        my @lines  = <PERSIST>;
        close PERSIST;

        chomp @lines;
        my $str = join "\n", @lines;
        return unless $str;

        eval {
            $persist = $json->decode( $str );
        };

        if ( $@ )
        {
            status 300;
            message "can't deserialize persistant data : $@";
            output 1;
        }
    }
    else
    {
        status 300;
        message "can't open persistant data file $path for reading : $!";
        output 1;
    }
}

1;
