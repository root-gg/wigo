/*
 * Charles-Antoine Mathieu <charles-antoine.mathieu@ovh.net>
 */

angular.module('dialog', ['ui.bootstrap']).
    factory('$dialog', function ($rootScope, $modal) {

        var module = {};

        // alert dialog
        module.alert = function (data) {
            if (!data) return false;
            var options = {
                backdrop: true,
                backdropClick: true,
                templateUrl: 'static/partials/alertDialogPartial.html',
                controller: 'AlertDialogController',
                resolve: {
                    args: function () {
                        return {
                            data: angular.copy(data)
                        }
                    }
                }
            };
            module.openDialog(options);
        };

        // generic dialog
        $rootScope.dialogs = [];
        module.openDialog = function (options) {
            if (!options) return false;

            $.each($rootScope.dialogs, function (i, dialog) {
                dialog.close();
            });
            $rootScope.dialogs = [];

            $modal.open(options);
        };

        return module;
    });

angular.module('wigo-refresh', []).
    factory('$refresh', function($timeout) {

        var module = {};

        module.init = function(refresh) {
            module.refresh = refresh;
            module.setRefreshInterval(60);
        }

        module.setRefreshInterval = function(interval) {
            if ( ! _.isUndefined(module.timeout) ) {
                $timeout.cancel(module.timeout);
            }
            if ( interval ) {
                module.interval = interval;
                module.callback = function() {
                    module.refresh();
                    module.timeout = $timeout(module.callback, module.interval * 1000);
                };
                module.timeout = $timeout(module.callback, module.interval * 1000);
            }
        }

        return module;
    });

angular.module('wigo-navigation', []).
    factory('$goto', function ($route, $location, $anchorScroll) {

        var module = {};

        module.group = function(group){
             $location.search('name',group);
             $location.path('group');
             $location.hash(null);
             $route.reload();
        }

        module.host = function(host){
             $location.search('name',host);
             $location.path('host');
             $location.hash(null);
             $route.reload();
        }

        module.probe = function(host,probe){
             $location.search('name',host);
             $location.path('host');
             $location.hash(probe);
             $route.reload();
        }

        module.anchor = function(anchor){
            $location.hash(anchor);
            $anchorScroll();
        }

        return module;
    });

var panelLevels = {
    "OK"        : "panel-success",
    "INFO"      : "panel-primary",
    "WARNING"   : "panel-warning",
    "CRITICAL"  : "panel-danger",
    "ERROR"     : "panel-error"
}

var labelLevels = {
    "OK"        : "label-success",
    "INFO"      : "label-primary",
    "WARNING"   : "label-warning",
    "CRITICAL"  : "label-danger",
    "ERROR"     : "label-error"
}

var badgeLevels = {
    "OK"        : "badge-success",
    "INFO"      : "badge-primary",
    "WARNING"   : "badge-warning",
    "CRITICAL"  : "badge-danger",
    "ERROR"     : "badge-error"
}

var btnLevels = {
    "OK"        : "btn-success",
    "INFO"      : "btn-primary",
    "WARNING"   : "btn-warning",
    "CRITICAL"  : "btn-danger",
    "ERROR"     : "btn-error"
}

var statusRowLevels = {
    "OK"        : "alert-success",
    "INFO"      : "alert-info",
    "WARNING"   : "alert-warning",
    "CRITICAL"  : "alert-danger",
    "ERROR"     : "alert-error"
}

var logLevels = [
    "DEBUG",
    "OK",
    "INFO",
    "ERROR",
    "WARNING",
    "CRITICAL",
    "EMERGENCY"
]

var logRowLevels = {
    "DEBUG"     : "",
    "OK"        : "alert-success",
    "INFO"      : "alert-info",
    "WARNING"   : "alert-warning",
    "CRITICAL"  : "alert-danger",
    "ERROR"     : "alert-error",
    "EMERGENCY" : "alert-error"
}

angular.module('wigo-filters', [])
    .filter('panelLevelCssFilter', function() {
        return function(level) {
            return panelLevels[level];
        };
    })
    .filter('labelLevelCssFilter', function() {
            return function(level) {
                return labelLevels[level];
            };
    })
    .filter('btnLevelCssFilter', function() {
            return function(level) {
                return btnLevels[level];
            };
    })
    .filter('badgeLevelCssFilter', function() {
            return function(level) {
                return badgeLevels[level];
            };
    })
    .filter('statusTableRowCssFilter', function() {
            return function(status) {
                return statusRowLevels[getLevel(status)];
            };
    })
    .filter('logLevelTableRowCssFilter', function() {
        return function(status) {
            return logRowLevels[logLevels[status - 1]];
        };
    })
    .filter('logLevelFilter', function() {
        return function(logs,minLevel){
            min = logLevels.indexOf(minLevel);
            return _.filter(logs, function(log){
                return log.Level >= min + 1;
            })
        };
    });

angular.module('wigo', ['ngRoute', 'dialog', 'restangular', 'wigo-filters', 'wigo-navigation', 'wigo-refresh'])
	.config(function($routeProvider) {
		$routeProvider
			.when('/',          { controller: HostsCtrl,        templateUrl:'partials/hosts.html',      reloadOnSearch: false })
			.when('/group',     { controller: GroupCtrl,        templateUrl:'partials/group.html',      reloadOnSearch: false })
			.when('/host',      { controller: HostCtrl,         templateUrl:'partials/host.html',       reloadOnSearch: false })
			.when('/logs',      { controller: LogsCtrl,         templateUrl:'partials/logs.html',       reloadOnSearch: false })
			.when('/authority', { controller: AuthorityCtrl,    templateUrl:'partials/authority.html',  reloadOnSearch: false })
			.otherwise({ redirectTo: '/' });
    })
    .config(function(RestangularProvider) {
        RestangularProvider.setBaseUrl('/api');
    });

function getLevel(status) {
    if ( status < 100 ) {
        return "ERROR";
    } else if (status == 100) {
        return "OK";
    } else if (status < 200) {
        return "INFO";
    } else if (status < 300) {
        return "WARNING";
    } else if (status <500) {
        return "CRITICAL";
    } else {
        return "ERROR";
    }
}

function HostsCtrl($scope, Restangular, $dialog, $route, $location, $anchorScroll, $timeout, $goto, $refresh) {

    $scope.init = function() {
        $scope.load();
        $scope.title = "Hosts";
    }

    $scope.load = function() {
        $scope.groups = [];
        $scope.counts = {
            "OK" : 0,
            "INFO" : 0,
            "WARNING" : 0,
            "CRITICAL" : 0,
            "ERROR" : 0
        };
        Restangular.all('groups').getList().then(function(groups) {
            _.each(groups,function(group_name){
                Restangular.one('groups',group_name).get().then(function(group){
                    group.counts = {
                        "OK" : 0,
                        "INFO" : 0,
                        "WARNING" : 0,
                        "CRITICAL" : 0,
                        "ERROR" : 0
                    };
                    group.Level = getLevel(group.Status);
                    _.each(group.Hosts,function(host){
                        host.Level = getLevel(host.Status);
                        $scope.counts[host.Level]++;
                        group.counts[host.Level]++;
                        _.each(host.Probes,function(probe){
                            probe.Level = getLevel(probe.Status);
                        });
                    });
                    $scope.groups.push(group);
                    $timeout($anchorScroll);
                });
            });
        });
    }

    $scope.goto = $goto;
    $scope.refresh = $refresh;

    $scope.init();
    $refresh.init($scope.load);
}

function GroupCtrl($scope, Restangular, $dialog, $route, $location, $anchorScroll, $timeout, $goto, $refresh) {

    $scope.init = function() {
        $scope.group = $location.search().name;
        $scope.title = 'Group: '+$scope.group;
        $scope.load();
    }

    $scope.load = function() {
        $scope.hosts = [];
        if (!$scope.group) return;
        $scope.counts = {
            "OK" : 0,
            "INFO" : 0,
            "WARNING" : 0,
            "CRITICAL" : 0,
            "ERROR" : 0
        };
        Restangular.one('groups',$scope.group).get().then(function(group) {
            _.each(group.Hosts,function(host){
                host.counts = {
                    "OK" : 0,
                    "INFO" : 0,
                    "WARNING" : 0,
                    "CRITICAL" : 0,
                    "ERROR" : 0
                };
                host.Level = getLevel(host.Status);
                _.each(host.Probes,function(probe){
                    probe.Level = getLevel(probe.Status);
                    $scope.counts[probe.Level]++;
                    host.counts[probe.Level]++;
                });
                $scope.hosts.push(host);
            });
            $timeout($anchorScroll);
        });
    }

    $scope.goto = $goto;
    $scope.refresh = $refresh;

    $scope.init();
    $refresh.init($scope.load);
}

function HostCtrl($scope, Restangular, $dialog, $route, $location, $anchorScroll, $timeout, $goto, $refresh) {
     $scope.init = function() {
        $scope.host = $location.search().name;
        $scope.title = 'Host: '+$scope.host;
        $scope.load();
    }

    $scope.load = function() {
        $scope.probes = [];
        if (!$scope.host) return;
        $scope.counts = {
            "OK" : 0,
            "INFO" : 0,
            "WARNING" : 0,
            "CRITICAL" : 0,
            "ERROR" : 0
        };
        Restangular.one('hosts',$scope.host).get().then(function(host) {
            _.each(host.LocalHost.Probes,function(probe){
                probe.Level = getLevel(probe.Status)
                $scope.counts[probe.Level]++;
                $scope.probes.push(probe);
            });

            $timeout($anchorScroll);
        });
    }

    $scope.goto = $goto;
    $scope.refresh = $refresh;

    $scope.init();
    $refresh.init($scope.load);
}

function LogsCtrl($scope, Restangular, $dialog, $route, $location, $goto, $refresh) {
    $scope.logLevels = logLevels;

    $scope.menu = {
        level : "OK"
    };

    $scope.load = function() {
        var _logs = Restangular;
        if($scope.menu.host){
            _logs = _logs.one('hosts',  $scope.menu.host)
        } else if($scope.menu.group){
            _logs = _logs.one('groups', $scope.menu.group)
        }
        if($scope.menu.probe){
            _logs = _logs.one('probes', $scope.menu.probe)
        }
        _logs = _logs.all('logs');

        _logs.getList({ offset:$scope.offset , limit:$scope.limit }).then(function(logs) {
            $scope.logs = logs;
        });

        var _indexes = Restangular.all('logs').one('indexes')
        _indexes.get().then(function(indexes){
            $scope.indexes = indexes;
        });
    }

    $scope.init = function() {
        $scope.title = 'Logs';
        $scope.menu.group    = $location.search().group;
        $scope.menu.host     = $location.search().host;
        $scope.menu.probe    = $location.search().probe;
        $scope.offset        = 0;
        $scope.limit         = 100;
        $scope.load();
    }

    $scope.updateUrl = function(){
        $location.search({
            group : $scope.menu.group,
            host : $scope.menu.host,
            probe : $scope.menu.probe,
        });
    }

    $scope.remove_group = function() {
        delete $scope.menu.group;
        $scope.load();
        $scope.updateUrl();
    }

    $scope.remove_host = function() {
        delete $scope.menu.host;
        $scope.load();
        $scope.updateUrl();
    }

    $scope.remove_probe = function() {
        delete $scope.menu.probe;
        $scope.load();
        $scope.updateUrl();
    }

    $scope.set_group = function(group) {
        $scope.menu.group = group;
        $scope.load();
        $scope.updateUrl();
    }

    $scope.set_host = function(host) {
        $scope.menu.host = host;
        $scope.load();
        $scope.updateUrl();
    }

    $scope.set_probe = function(probe) {
        $scope.menu.probe = probe;
        $scope.load();
        $scope.updateUrl();
    }

    $scope.prev = function() {
        if ( $scope.offset < $scope.limit ) return;
        $scope.offset -= $scope.limit;
        $scope.load();
    }

    $scope.next = function() {
        $scope.offset += $scope.limit;
        $scope.load();
    }

    $scope.goto = $goto;
    $scope.refresh = $refresh;

    $scope.init();
    $refresh.init($scope.load);
}


function AuthorityCtrl($scope, Restangular, $dialog, $route, $location, $goto, $q,$refresh) {

    $scope.init = function() {
        $scope.title = "Allowed clients";
        $scope.load();
    }

    $scope.load = function() {
        $scope.waiting = [];
        $scope.allowed = [];
        Restangular.one('authority').one('hosts').get().then(function(hosts) {
            _.each(hosts.waiting, function(hostname,uuid) {
                $scope.waiting.push({ uuid : uuid, hostname : hostname });
            });
            _.each(hosts.allowed, function(hostname,uuid) {
                $scope.allowed.push({ uuid : uuid, hostname : hostname });
            });
        });
    }

    $scope.allow = function(host) {
        Restangular.one('authority').one('hosts',host.uuid).one('allow').post().then(function() {
            $scope.load();
        })
    }

    $scope.allow_all = function(host) {
        var requests = [];
        _.each($scope.waiting, function(host) {
            requests.push(Restangular.one('authority').one('hosts',host.uuid).one('allow').post())
        });
        $q.all(requests).then(function() {
            $scope.load();
        })
    }

    $scope.revoke = function(host) {
        Restangular.one('authority').one('hosts',host.uuid).one('revoke').post().then(function() {
            $scope.load();
        })
    }

    $scope.revoke_all = function() {
        var requests = [];
        _.each($scope.allowed, function(host) {
            requests.push(Restangular.one('authority').one('hosts',host.uuid).one('revoke').post())
        });
        $q.all(requests).then(function() {
            $scope.load();
        })
    }

    $scope.refresh = $refresh;

    $scope.init();
    $refresh.init($scope.load);
}

function AlertDialogController($rootScope, $scope, $modalInstance, args) {
    $rootScope.dialogs.push($scope);

    $scope.title = 'Success !';
    if (args.data.status != 100) $scope.title = 'Oops !';

    $scope.data = args.data;

    $scope.close = function (result) {
        $rootScope.dialogs = _.without($rootScope.dialogs, $scope);
        $modalInstance.close(result);
        if(args.callback) {
            args.callback(result);
        }
    };
}
